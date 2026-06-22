package cli

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/webmention"
)

// WebmentionCmd groups the webmention operations. Sending is implemented; fetching/
// publishing received mentions (display) are the designed Tier-2 layer (docs/design/webmention.md).
type WebmentionCmd struct {
	Send    WebmentionSendCmd    `cmd:"" help:"Notify the sites your live posts link to (run after publish)"`
	Fetch   WebmentionFetchCmd   `cmd:"" help:"Pull received mentions from the configured receiver into the local cache"`
	Publish WebmentionPublishCmd `cmd:"" help:"Fetch mentions and deploy only _mentions/ (refresh responses without a full re-upload)"`
}

// WebmentionPublishCmd refreshes received mentions on a deployed site without re-uploading the
// whole thing: it fetches into the cache, then deploys only the _mentions/ prefix. Run it on a
// schedule (cron) so asset-mode responses stay fresh between content builds.
type WebmentionPublishCmd struct {
	Env          string `help:"Environment to refresh mentions on" default:"production"`
	AllowPublish bool   `help:"Deploy environments that set allow_publish: false"`
	Domain       string `help:"Domain to fetch mentions for (default: the site base_url host)"`
	Verbose      bool   `short:"v" help:"Log each step"`
}

func (c *WebmentionPublishCmd) Run() error {
	fetch := &WebmentionFetchCmd{Domain: c.Domain, Verbose: c.Verbose}
	if err := fetch.Run(); err != nil {
		return err
	}
	pub := &PublishCmd{Env: []string{c.Env}, AllowPublish: c.AllowPublish, Verbose: c.Verbose, mentionsOnly: true}
	return pub.Run()
}

// WebmentionFetchCmd reads received mentions back from the configured receiver's read API
// (the jf2 driver) and fully regenerates the local cache (.colophon/cache/webmentions/), so
// deletions self-heal. The build (asset mode) and `webmention publish` read that cache.
type WebmentionFetchCmd struct {
	Domain  string `help:"Domain to fetch mentions for (default: the site base_url host)"`
	Verbose bool   `short:"v" help:"Log each post that received mentions"`
}

func (c *WebmentionFetchCmd) Run() error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	cfg, err := config.Load(root)
	if err != nil {
		return err
	}
	log := newLogger(c.Verbose)
	if len(cfg.Sites) == 0 {
		return fmt.Errorf("no sites configured")
	}
	site := cfg.Sites[0]

	wm := webmentionConf(site)
	if wm == nil {
		return fmt.Errorf("no federation.indieweb.webmention configured")
	}
	endpoint := webmention.ReadEndpoint(wm.Source, wm.Receiver)
	if endpoint == "" {
		return fmt.Errorf("set federation.indieweb.webmention.receiver (or .source) so the read API can be derived")
	}
	domain := c.Domain
	if domain == "" {
		if u, err := url.Parse(site.BaseURL); err == nil {
			domain = u.Host
		}
	}
	if domain == "" {
		return fmt.Errorf("could not determine domain; set --domain or the site base_url")
	}
	if strings.TrimSpace(wm.Token) == "" {
		log.Step("WEBMENTION", "fetch", "warning",
			"no token set — webmention.io domain reads need federation.indieweb.webmention.token: {env:WEBMENTION_IO_TOKEN}")
	}

	data, err := webmention.FetchJF2(context.Background(), webmention.DefaultClient, endpoint, domain, wm.Token)
	if err != nil {
		return err
	}

	// Moderation chokepoint: drop blocklisted mentions before they reach the cache, so the filter
	// survives the full regenerate (editing the generated JSON would just be overwritten).
	block, err := webmention.LoadBlocklist(root)
	if err != nil {
		return fmt.Errorf("load blocklist: %w", err)
	}

	// Full regenerate: clear the namespace, then write the current set. A post that lost all its
	// mentions ends up with no file (so display renders nothing) rather than a stale leftover.
	dir := webmention.CacheDir(root)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clear mentions cache: %w", err)
	}
	total, blocked, posts := 0, 0, 0
	for key, m := range data {
		before := len(m.Mentions)
		m.Mentions = block.Filter(m.Mentions)
		blocked += before - len(m.Mentions)
		if len(m.Mentions) == 0 {
			continue // nothing left after moderation → no file for this post
		}
		if err := webmention.SaveCached(dir, key, m); err != nil {
			return err
		}
		posts++
		total += len(m.Mentions)
		if c.Verbose {
			log.Detail("WEBMENTION", m.Target, "mentions", len(m.Mentions))
		}
	}
	log.Step("WEBMENTION", "fetch", "domain", domain, "posts", posts, "mentions", total, "blocked", blocked)
	return nil
}

// webmentionConf returns the site's webmention config, or nil.
func webmentionConf(site core.Site) *core.WebmentionConf {
	if iw := site.Federation.IndieWeb; iw != nil {
		return iw.Webmention
	}
	return nil
}

// WebmentionSendCmd scans a built environment's HTML for outbound links and sends a
// webmention to each target's endpoint, so your post shows up as a response there. It works
// off the already-built output (run it after publish, when the source URLs are live).
type WebmentionSendCmd struct {
	Env     string `help:"Environment whose built output to scan" default:"production"`
	DryRun  bool   `name:"dry-run" help:"Discover endpoints and report, but do not POST"`
	Verbose bool   `short:"v" help:"Log each link and endpoint"`
}

func (c *WebmentionSendCmd) Run() error {
	root, err := findRoot()
	if err != nil {
		return err
	}
	log := newLogger(c.Verbose)

	buildDir := filepath.Join(root, ".colophon", "build", c.Env)
	if fi, err := os.Stat(buildDir); err != nil || !fi.IsDir() {
		return fmt.Errorf("no build for environment %q at %s — run `colophon publish --env %s` (or `build`) first",
			c.Env, buildDir, c.Env)
	}

	// Gather outbound links per source page from the built HTML. The source URL is the page's
	// baked canonical (the authoritative live URL); pages without one can't be a source.
	bySource, err := collectOutbound(buildDir)
	if err != nil {
		return err
	}
	if len(bySource) == 0 {
		log.Step("WEBMENTION", "send", "result", "no outbound links found")
		return nil
	}

	cachePath := filepath.Join(root, ".colophon", "cache", "webmention-sent.json")
	cache, err := webmention.LoadSentCache(cachePath)
	if err != nil {
		return fmt.Errorf("load sent cache: %w", err)
	}

	ctx := context.Background()
	endpoints := map[string]string{} // target → endpoint, discovered once per run
	var sent, failed, skipped int

	for source, targets := range bySource {
		if localBaseURL(source) {
			log.Step("WEBMENTION", source, "skipped", "source URL is local (receiver cannot fetch it back)")
			continue
		}
		added, dropped := webmention.Diff(cache.Sent[source], targets)
		for _, target := range append(added, dropped...) {
			ep, ok := endpoints[target]
			if !ok {
				ep, err = webmention.Discover(ctx, webmention.DefaultClient, target)
				if err != nil {
					log.Detail("WEBMENTION", target, "discover", "error", "error", err.Error())
				}
				endpoints[target] = ep
			}
			if ep == "" {
				skipped++
				log.Detail("WEBMENTION", target, "endpoint", "none (target does not accept webmentions)")
				continue
			}
			if c.DryRun {
				log.Step("WEBMENTION", "would-send", "source", source, "target", target, "endpoint", ep)
				sent++
				continue
			}
			if err := webmention.Send(ctx, webmention.DefaultClient, ep, source, target); err != nil {
				failed++
				log.Step("WEBMENTION", "send", "source", source, "target", target, "status", "failed", "error", err.Error())
				continue
			}
			sent++
			log.Step("WEBMENTION", "send", "source", source, "target", target, "status", "ok")
		}
		cache.Sent[source] = targets
	}

	if c.DryRun {
		log.Step("WEBMENTION", "send", "dry_run", true, "would_send", sent, "no_endpoint", skipped)
		return nil
	}
	if err := cache.Save(cachePath); err != nil {
		return fmt.Errorf("save sent cache: %w", err)
	}
	log.Step("WEBMENTION", "send", "sent", sent, "failed", failed, "no_endpoint", skipped)
	return nil
}

// collectOutbound walks the build dir's HTML and returns each page's outbound (cross-origin)
// links keyed by the page's canonical source URL.
func collectOutbound(buildDir string) (map[string][]string, error) {
	out := map[string][]string{}
	err := filepath.WalkDir(buildDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".html") {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source := webmention.CanonicalURL(body)
		if source == "" {
			return nil // no canonical → can't be a webmention source
		}
		links, err := webmention.OutboundLinks(source, body)
		if err != nil || len(links) == 0 {
			return err
		}
		out[source] = links
		return nil
	})
	return out, err
}
