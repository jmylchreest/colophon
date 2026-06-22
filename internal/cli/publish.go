package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmylchreest/colophon/internal/build"
	"github.com/jmylchreest/colophon/internal/clog"
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/profiling"
	"github.com/jmylchreest/colophon/internal/publish"
	"github.com/jmylchreest/colophon/internal/telemetry"
	"github.com/jmylchreest/colophon/internal/websub"
)

// PublishCmd builds and deploys one or more environments. Each --env is built with its
// own overrides and deployed to its publishers; a gated environment (allow_publish:
// false) is skipped unless --allow-publish is passed.
type PublishCmd struct {
	Env          []string `help:"Environment to publish; repeat for several" required:""`
	AllowPublish bool     `help:"Deploy environments that set allow_publish: false"`
	Create       bool     `help:"Create the destination (e.g. a Pages project) if it doesn't exist"`
	GenerateAI   bool     `name:"generate-ai" help:"Generate uncached AI media (gen: images and TTS audio) via the configured providers before deploying"`
	Verbose      bool     `short:"v" help:"Log each step (sources, files, publisher actions)"`
	Pprof        string   `help:"Capture CPU+heap profiles to a dir (or 1 for cwd)" hidden:""`
}

func (c *PublishCmd) Run() error {
	defer profiling.Capture(c.Pprof)()
	root, err := findRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	log := newLogger(c.Verbose)

	// Drop build trees of environments that no longer exist (keyed on the full set, not
	// just the ones being published now, so unrelated envs are preserved).
	keep := make(map[string]bool, len(cfg.Environments))
	for _, name := range cfg.EnvironmentNames() {
		keep[name] = true
	}
	if err := build.ReconcileDirs(filepath.Join(root, ".colophon", "build"), keep); err != nil {
		log.Step("PUBLISH", "", "cleanup_warning", err.Error())
	}

	ctx := context.Background()
	var summary []summaryRow
	for _, name := range c.Env {
		if err := c.publishEnv(ctx, root, cfg, name, log, &summary); err != nil {
			return err
		}
	}
	for _, r := range summary {
		kv := []any{}
		if r.publisher != "" {
			kv = append(kv, "publisher", r.publisher)
		}
		kv = append(kv, "status", r.status)
		if r.url != "" {
			kv = append(kv, "url", r.url)
		}
		if r.status == "deployed" {
			kv = append(kv, "files", r.files, "uploaded", r.uploaded)
		}
		log.Step("SUMMARY", r.env, kv...)
	}
	return nil
}

// summaryRow is one line of the end-of-run recap: where each environment went, and how.
type summaryRow struct {
	env, publisher, status, url string
	files, uploaded             int
}

// localBaseURL reports whether a base_url is empty or a loopback address — unsuitable
// for the canonical URLs baked into feeds/sitemap on a real deploy.
func localBaseURL(u string) bool {
	if u == "" {
		return true
	}
	for _, s := range []string{"localhost", "127.0.0.1", "[::1]", "://0.0.0.0"} {
		if strings.Contains(u, s) {
			return true
		}
	}
	return false
}

// hasRemote reports whether the environment deploys to any non-local publisher.
func hasRemote(cfg *config.Config, env *config.Environment) bool {
	for _, id := range env.Publish {
		if pc := cfg.Publisher(id); pc != nil && pc.Driver != "local" {
			return true
		}
	}
	return false
}

// deployTarget is one opened publisher for an environment, with its config id and driver.
type deployTarget struct {
	id     string
	driver string
	pub    core.Publisher
}

// publishEnv builds one environment and deploys it. It orchestrates the focused steps —
// openTargets, provisionTargets, resolveBaseURLs, build, deployAll, writeManifests — so each
// piece stays small and the precedence/gating rules read top-to-bottom.
func (c *PublishCmd) publishEnv(ctx context.Context, root string, cfg *config.Config, name string, log *clog.Logger, summary *[]summaryRow) error {
	env := cfg.Environment(name)
	if env == nil {
		return unknownEnvErr(cfg, name)
	}
	log.Step("ENV", name, "publish", strings.Join(env.Publish, ","), "drafts", env.IncludeDrafts)

	tel := telemetryFor(cfg, name, root)
	defer tel.Flush()

	targets, err := openTargets(root, cfg, env, name, log)
	if err != nil {
		return err
	}
	gated := env.Gated() && !c.AllowPublish

	// Provisioning and URL discovery may hit the network and only matter for a real deploy,
	// so both are skipped when gated.
	if c.Create && !gated {
		if err := provisionTargets(ctx, targets, name, log); err != nil {
			return err
		}
	}
	baseURL, routes := resolveBaseURLs(ctx, cfg, env, targets, gated, name, log)

	outDir := filepath.Join(root, ".colophon", "build", name)
	if _, err := build.Run(cfg, build.Options{
		OutDir:        outDir,
		IncludeDrafts: env.IncludeDrafts,
		Title:         env.Title,
		BaseURL:       baseURL,
		Theme:         env.Theme,
		Publishers:    env.Publish,
		Routes:        routes,
		Log:           log,
		Env:           name,
		Telemetry:     tel,
		GenerateAI:    c.GenerateAI,
	}); err != nil {
		return err
	}
	log.Detail("BUILD", "", "out", outDir)

	if gated {
		log.Step("PUBLISH", "", "env", name, "skipped", "allow_publish=false")
		*summary = append(*summary, summaryRow{env: name, publisher: strings.Join(env.Publish, ","), status: "gated"})
		return nil
	}

	siteURL := ""
	if !localBaseURL(baseURL) {
		siteURL = baseURL
	}
	if hasRemote(cfg, env) && localBaseURL(baseURL) {
		log.Step("PUBLISH", "", "env", name, "warning", "base_url is local; feeds/sitemap will be non-public")
	}

	if err := deployAll(ctx, env, name, targets, routes, outDir, siteURL, log, summary, tel); err != nil {
		return err
	}
	writeManifests(ctx, cfg, targets, name, siteURL, log)
	pingWebSubHubs(ctx, cfg, siteURL, log)
	return nil
}

// pingWebSubHubs notifies the configured WebSub hubs that the site's feeds changed, after
// a successful deploy. Best-effort: the feed must be live (so it needs a public siteURL),
// and a failed ping only logs — it never fails the publish.
func pingWebSubHubs(ctx context.Context, cfg *config.Config, siteURL string, log *clog.Logger) {
	if siteURL == "" || len(cfg.Sites) == 0 {
		return
	}
	site := cfg.Sites[0]
	if site.Federation.WebSub == nil || len(site.Federation.WebSub.Hubs) == 0 {
		return
	}
	feeds := build.FeedURLs(site, siteURL)
	for _, hub := range site.Federation.WebSub.Hubs {
		hub = strings.TrimSpace(hub)
		if hub == "" {
			continue
		}
		for _, topic := range feeds {
			if err := websub.Ping(ctx, websub.DefaultClient, hub, topic); err != nil {
				log.Step("WEBSUB", hub, "topic", topic, "ping", "failed", "error", err.Error())
				continue
			}
			log.Detail("WEBSUB", hub, "topic", topic, "ping", "ok")
		}
	}
}

// openTargets opens every publisher the environment deploys to, wiring each a logger if it
// wants one. It fails fast on an unknown publisher id.
func openTargets(root string, cfg *config.Config, env *config.Environment, name string, log *clog.Logger) ([]deployTarget, error) {
	var targets []deployTarget
	for _, pubID := range env.Publish {
		pc := cfg.Publisher(pubID)
		if pc == nil {
			return nil, fmt.Errorf("environment %q references unknown publisher %q", name, pubID)
		}
		pub, err := publish.Open(root, pc.Merged(env.Overrides[pubID]))
		if err != nil {
			return nil, err
		}
		if a, ok := pub.(clog.Aware); ok {
			a.SetLogger(log)
		}
		targets = append(targets, deployTarget{pubID, pc.Driver, pub})
	}
	return targets, nil
}

// provisionTargets creates any destination that supports it (e.g. a Pages project / R2
// bucket) before deploying. It also lets a freshly created project report its canonical URL.
func provisionTargets(ctx context.Context, targets []deployTarget, name string, log *clog.Logger) error {
	for _, t := range targets {
		prov, ok := t.pub.(core.Provisioner)
		if !ok {
			continue
		}
		created, err := prov.Provision(ctx)
		if err != nil {
			return fmt.Errorf("create %s: %w", t.id, err)
		}
		if created {
			log.Step("PUBLISH", t.id, "env", name, "created", true)
		}
	}
	return nil
}

// canonicalURLs memoises each target's CanonicalURL across one publish run: the env-level
// base_url and every routing rule that points at the same publisher reuse the cached value
// instead of re-hitting the backend (e.g. the Cloudflare API) once per lookup. A failed or
// empty resolution is cached too, so it is attempted at most once per publisher.
type canonicalURLs struct {
	ctx  context.Context
	name string
	log  *clog.Logger
	done map[string]string // publisher id → resolved URL ("" = tried, none)
}

func (uc *canonicalURLs) get(t deployTarget) (string, bool) {
	if u, ok := uc.done[t.id]; ok {
		return u, u != ""
	}
	u := ""
	if cu, ok := t.pub.(core.CanonicalURLer); ok {
		if got, err := cu.CanonicalURL(uc.ctx); err != nil {
			uc.log.Step("PUBLISH", t.id, "env", uc.name, "warning", "public URL discovery failed: "+err.Error())
		} else {
			u = got
		}
	}
	uc.done[t.id] = u
	return u, u != ""
}

// resolveBaseURLs resolves the canonical base_url for feeds/sitemap and the per-route asset
// base_urls. The env base_url precedence is: 1) env config 2) a publisher's CanonicalURL
// 3) the site default. Each routing rule with an empty base_url inherits its target
// publisher's public URL. (serve overrides all of this with its listening address.)
func resolveBaseURLs(ctx context.Context, cfg *config.Config, env *config.Environment, targets []deployTarget, gated bool, name string, log *clog.Logger) (string, []core.RouteRule) {
	uc := &canonicalURLs{ctx: ctx, name: name, log: log, done: map[string]string{}}

	baseURL := env.BaseURL
	if baseURL == "" && !gated {
		for _, t := range targets {
			if u, ok := uc.get(t); ok {
				baseURL = u
				log.Step("PUBLISH", t.id, "env", name, "base_url", u, "via", t.driver)
				break
			}
		}
	}
	if baseURL == "" && len(cfg.Sites) > 0 {
		baseURL = cfg.Sites[0].BaseURL
	}

	var routes []core.RouteRule
	if len(cfg.Sites) > 0 {
		routes = append(routes, cfg.Sites[0].Routing...)
	}
	if !gated {
		byID := make(map[string]deployTarget, len(targets))
		for _, t := range targets {
			byID[t.id] = t
		}
		for i := range routes {
			if strings.TrimSpace(routes[i].BaseURL) != "" {
				continue
			}
			t, ok := byID[routes[i].Publisher]
			if !ok {
				continue
			}
			if u, ok := uc.get(t); ok {
				routes[i].BaseURL = u
				log.Step("PUBLISH", t.id, "env", name, "asset_base_url", u)
			}
		}
	}
	return baseURL, routes
}

// deployAll deploys the built tree to each target, applying routing so each publisher sees
// only the files it owns, then prunes old deployments where supported. It appends a summary
// row per target.
func deployAll(ctx context.Context, env *config.Environment, name string, targets []deployTarget, routes []core.RouteRule, outDir, siteURL string, log *clog.Logger, summary *[]summaryRow, tel *telemetry.Client) error {
	base := os.DirFS(outDir)
	router := core.NewRouter(routes, env.Publish)
	routeTargets := map[string]bool{}
	for _, r := range routes {
		routeTargets[r.Publisher] = true
	}
	for _, t := range targets {
		// Routing partitions the tree: each publisher sees only the files it owns (a route
		// target gets its matched paths; a default publisher gets the unrouted remainder).
		// A route target whose routing never activated (e.g. no public URL was resolvable) is
		// skipped, so we never mirror the whole site into an assets-only bucket.
		if routeDecision(router.Active(), router.Owns(t.id), routeTargets[t.id]) == deliverSkip {
			log.Step("PUBLISH", t.id, "env", name, "skipped",
				"no public URL resolved for routed assets; they remain co-located on the other publisher(s)")
			*summary = append(*summary, summaryRow{env: name, publisher: t.id, status: "skipped"})
			tel.Publish(t.driver, t.id, "skipped", 0, 0)
			continue
		}
		tree := base
		if router.Active() {
			id := t.id
			tree = selectFS{base: base, keep: func(p string) bool { return router.Keep(id, p) }}
			log.Detail("PUBLISH", t.id, "env", name, "routed", router.Owns(t.id))
		}
		// A whole-tree destination (git branch, external deploy command) takes the entire tree
		// at once — no incremental plan — so it is dispatched to Push instead of publish.Run.
		var result core.Result
		var err error
		if gp, ok := t.pub.(core.TreePublisher); ok {
			result, err = gp.Push(ctx, tree)
		} else {
			result, err = publish.Run(ctx, tree, t.pub)
		}
		if err != nil {
			return fmt.Errorf("deploy %s: %w", t.id, err)
		}
		log.Step("PUBLISH", t.id, "env", name, "files", result.Total,
			"uploaded", result.Uploaded, "deleted", result.Deleted, "bytes", result.Bytes,
			"unchanged", result.Total-result.Uploaded)
		if result.URL != "" {
			log.Step("PUBLISH", t.id, "env", name, "url", result.URL)
		}
		*summary = append(*summary, summaryRow{
			env: name, publisher: t.id, status: "deployed",
			url: siteURL, files: result.Total, uploaded: result.Uploaded,
		})
		tel.Publish(t.driver, t.id, "deployed", result.Uploaded, result.Total)
		if pruner, ok := t.pub.(core.Pruner); ok {
			// Best-effort: the deploy already succeeded, so a prune error only warns.
			if removed, err := pruner.Prune(ctx); err != nil {
				log.Step("PUBLISH", t.id, "env", name, "prune_warning", err.Error())
			} else if removed > 0 {
				log.Step("PUBLISH", t.id, "env", name, "pruned", removed)
			}
		}
	}
	return nil
}

// writeManifests records provenance on publishers that support it (e.g. an object store
// writing .well-known/colophon.json), so a destination names its blog and links back to the
// canonical site. Only with a public site URL, and best-effort (a failure only warns).
func writeManifests(ctx context.Context, cfg *config.Config, targets []deployTarget, name, siteURL string, log *clog.Logger) {
	if siteURL == "" || len(cfg.Sites) == 0 {
		return
	}
	manifest := core.SiteManifest{
		Generator: "colophon",
		Site:      siteURL,
		Sitemap:   strings.TrimRight(siteURL, "/") + "/sitemap.xml",
		Feeds:     build.FeedURLs(cfg.Sites[0], siteURL),
	}
	for _, t := range targets {
		mw, ok := t.pub.(core.ManifestWriter)
		if !ok {
			continue
		}
		if err := mw.WriteManifest(ctx, manifest); err != nil {
			log.Step("PUBLISH", t.id, "env", name, "manifest_warning", err.Error())
		} else {
			log.Detail("PUBLISH", t.id, "env", name, "manifest", "written")
		}
	}
}
