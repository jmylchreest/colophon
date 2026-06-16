package cli

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmylchreest/colophon/internal/build"
	"github.com/jmylchreest/colophon/internal/clog"
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/publish"
)

// PublishCmd builds and deploys one or more environments. Each --env is built with its
// own overrides and deployed to its publishers; a gated environment (allow_publish:
// false) is skipped unless --allow-publish is passed.
type PublishCmd struct {
	Env          []string `help:"Environment to publish; repeat for several" required:""`
	AllowPublish bool     `help:"Deploy environments that set allow_publish: false"`
	Create       bool     `help:"Create the destination (e.g. a Pages project) if it doesn't exist"`
	Verbose      bool     `short:"v" help:"Log each step (sources, files, publisher actions)"`
}

func (c *PublishCmd) Run() error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	log := newLogger(cfg, c.Verbose)

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

func (c *PublishCmd) publishEnv(ctx context.Context, root string, cfg *config.Config, name string, log *clog.Logger, summary *[]summaryRow) error {
	env := cfg.Environment(name)
	if env == nil {
		return unknownEnvErr(cfg, name)
	}
	log.Step("ENV", name, "publish", strings.Join(env.Publish, ","), "drafts", env.IncludeDrafts)

	// Open this environment's publishers up front; hand each a logger if it wants one.
	type deployTarget struct {
		id     string
		driver string
		pub    core.Publisher
	}
	var targets []deployTarget
	for _, pubID := range env.Publish {
		pc := cfg.Publisher(pubID)
		if pc == nil {
			return fmt.Errorf("environment %q references unknown publisher %q", name, pubID)
		}
		pub, err := publish.Open(root, pc.Merged(env.Overrides[pubID]))
		if err != nil {
			return err
		}
		if a, ok := pub.(clog.Aware); ok {
			a.SetLogger(log)
		}
		targets = append(targets, deployTarget{pubID, pc.Driver, pub})
	}

	gated := env.Gated() && !c.AllowPublish

	// Provision destinations first — also lets a publisher report its canonical URL for
	// a freshly created project. Skipped when gated, since we won't deploy.
	if c.Create && !gated {
		for _, t := range targets {
			if prov, ok := t.pub.(core.Provisioner); ok {
				created, err := prov.Provision(ctx)
				if err != nil {
					return fmt.Errorf("create %s: %w", t.id, err)
				}
				if created {
					log.Step("PUBLISH", t.id, "env", name, "created", true)
				}
			}
		}
	}

	// Resolve base_url for feeds/sitemap by precedence:
	//   1. env-defined config  2. provider CanonicalURL  3. site default
	// (serve overrides all of this with its listening address.)
	baseURL := env.BaseURL
	if baseURL == "" && !gated {
		for _, t := range targets {
			cu, ok := t.pub.(core.CanonicalURLer)
			if !ok {
				continue
			}
			if u, err := cu.CanonicalURL(ctx); err == nil && u != "" {
				baseURL = u
				log.Step("PUBLISH", t.id, "env", name, "base_url", u, "via", t.driver)
				break
			}
		}
	}
	if baseURL == "" && len(cfg.Sites) > 0 {
		baseURL = cfg.Sites[0].BaseURL
	}

	// Resolve each routing rule's base_url: an empty one inherits its target publisher's
	// public URL (e.g. a discovered R2 custom/managed domain), so it needn't be repeated in
	// config. Skipped when gated, since we won't deploy and resolution may hit the network.
	var routes []core.RouteRule
	if len(cfg.Sites) > 0 {
		routes = append(routes, cfg.Sites[0].Routing...)
	}
	if !gated {
		for i := range routes {
			if strings.TrimSpace(routes[i].BaseURL) != "" {
				continue
			}
			for _, t := range targets {
				cu, ok := t.pub.(core.CanonicalURLer)
				if t.id != routes[i].Publisher || !ok {
					continue
				}
				u, err := cu.CanonicalURL(ctx)
				switch {
				case err != nil:
					log.Step("PUBLISH", t.id, "env", name, "warning", "public URL discovery failed: "+err.Error())
				case u != "":
					routes[i].BaseURL = u
					log.Step("PUBLISH", t.id, "env", name, "asset_base_url", u)
				}
			}
		}
	}

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
			continue
		}
		tree := fs.FS(base)
		if router.Active() {
			id := t.id
			tree = selectFS{base: base, keep: func(p string) bool { return router.Keep(id, p) }}
			log.Detail("PUBLISH", t.id, "env", name, "routed", router.Owns(t.id))
		}
		changes, err := t.pub.Plan(ctx, tree)
		if err != nil {
			return fmt.Errorf("plan %s: %w", t.id, err)
		}
		result, err := t.pub.Apply(ctx, tree, changes)
		if err != nil {
			return fmt.Errorf("apply %s: %w", t.id, err)
		}
		log.Step("PUBLISH", t.id, "env", name, "files", result.Total,
			"uploaded", result.Uploaded, "bytes", result.Bytes, "unchanged", result.Total-result.Uploaded)
		if result.URL != "" {
			log.Step("PUBLISH", t.id, "env", name, "url", result.URL)
		}
		*summary = append(*summary, summaryRow{
			env: name, publisher: t.id, status: "deployed",
			url: siteURL, files: result.Total, uploaded: result.Uploaded,
		})
		if pruner, ok := t.pub.(core.Pruner); ok {
			// Best-effort: the deploy already succeeded, so a prune error only warns.
			if removed, err := pruner.Prune(ctx); err != nil {
				log.Step("PUBLISH", t.id, "env", name, "prune_warning", err.Error())
			} else if removed > 0 {
				log.Step("PUBLISH", t.id, "env", name, "pruned", removed)
			}
		}
	}

	// Record provenance on publishers that support it (e.g. an object store writing
	// .well-known/colophon.json), so a destination names its blog and links back to the
	// canonical site. Only with a public site URL, and best-effort.
	if siteURL != "" && len(cfg.Sites) > 0 {
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
	return nil
}
