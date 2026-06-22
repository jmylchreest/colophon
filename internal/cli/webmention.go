package cli

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmylchreest/colophon/internal/webmention"
)

// WebmentionCmd groups the webmention operations. Sending is implemented; fetching/
// publishing received mentions (display) are the designed Tier-2 layer (docs/design/webmention.md).
type WebmentionCmd struct {
	Send WebmentionSendCmd `cmd:"" help:"Notify the sites your live posts link to (run after publish)"`
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
