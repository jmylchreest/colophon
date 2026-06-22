package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmylchreest/colophon/internal/build"
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/syndicate"
)

// SyndicateCmd cross-posts (POSSE) eligible posts to the environment's configured syndicators,
// recording each in the committed ledger so re-runs never double-post. It performs irreversible
// external actions, so it is fenced: only an env's `syndicate:` targets fire, a gated env needs
// --allow-publish, and a missing ledger refuses to run blind (use --dry-run to preview).
type SyndicateCmd struct {
	Env          string `help:"Environment to syndicate" default:"production"`
	AllowPublish bool   `help:"Run for environments gated by allow_publish: false (required to post)"`
	DryRun       bool   `name:"dry-run" help:"Show what would be syndicated; post nothing, write no ledger"`
	Verbose      bool   `short:"v" help:"Log each candidate"`
}

func (c *SyndicateCmd) Run() error {
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

	env := cfg.Environment(c.Env)
	if env == nil {
		return unknownEnvErr(cfg, c.Env)
	}
	allowed := env.Syndicate
	if len(allowed) == 0 {
		log.Step("SYNDICATE", c.Env, "result", "no syndicate: targets for this environment — nothing to do")
		return nil
	}
	// Posting is irreversible, so a gated env (allow_publish: false — typically production) needs
	// the explicit latch, exactly like deploy.
	if env.Gated() && !c.AllowPublish && !c.DryRun {
		return fmt.Errorf("environment %q is gated (allow_publish: false); pass --allow-publish to syndicate (or --dry-run to preview)", c.Env)
	}

	// Open the configured syndicators named by the env (and only those).
	open := map[string]syndicate.Syndicator{}
	for _, sc := range site.Federation.Syndication {
		if !contains(allowed, sc.ID) {
			continue
		}
		s, err := syndicate.Open(sc)
		if err != nil {
			return err
		}
		open[sc.ID] = s
	}
	for _, id := range allowed {
		if _, ok := open[id]; !ok {
			return fmt.Errorf("environment %q syndicates to %q, but no federation.syndication entry has that id", c.Env, id)
		}
	}

	ledger, existed, err := syndicate.LoadLedger(root)
	if err != nil {
		return fmt.Errorf("load ledger: %w", err)
	}
	// Blind-run guard: no ledger + a real run would re-post the whole back catalogue. Require the
	// explicit latch so a fresh runner that lost the committed ledger can't spam every silo.
	if !existed && !c.DryRun && !c.AllowPublish {
		return fmt.Errorf("no syndication ledger (%s) — a real run would post ALL eligible posts. Commit the ledger, or pass --allow-publish to seed it (or --dry-run to preview)", syndicate.LedgerPath(root))
	}

	entries, err := build.Entries(cfg)
	if err != nil {
		return err
	}
	base := site.BaseURL
	if env.BaseURL != "" {
		base = env.BaseURL
	}
	now := time.Now().UTC().Format(time.RFC3339)

	posted, skipped, failed := 0, 0, 0
	for _, e := range entries {
		if e.Type != "post" || e.Draft || e.SyndicateOff {
			continue
		}
		targets := syndicateTargets(e.SyndicateTargets, allowed)
		if len(targets) == 0 {
			continue
		}
		post := syndicate.Post{
			Key:       strings.Trim(e.URL, "/"),
			Title:     e.Title,
			URL:       strings.TrimRight(base, "/") + "/" + e.URL,
			Summary:   e.Description,
			Text:      e.SyndicateText,
			Tags:      e.Tags,
			Published: stampDate(e.Date),
		}
		for _, id := range targets {
			if ledger.Has(post.Key, id) {
				skipped++
				continue
			}
			if c.DryRun {
				log.Step("SYNDICATE", "would-post", "post", post.URL, "to", id)
				posted++
				continue
			}
			url, err := open[id].Syndicate(context.Background(), post)
			if err != nil {
				failed++
				log.Step("SYNDICATE", "post", "url", post.URL, "to", id, "status", "failed", "error", err.Error())
				continue
			}
			ledger.Set(post.Key, id, url, now)
			posted++
			log.Step("SYNDICATE", "post", "url", post.URL, "to", id, "silo", url, "status", "ok")
		}
	}

	if c.DryRun {
		log.Step("SYNDICATE", c.Env, "dry_run", true, "would_post", posted, "already", skipped)
		return nil
	}
	if err := ledger.Save(); err != nil {
		return fmt.Errorf("save ledger: %w", err)
	}
	log.Step("SYNDICATE", c.Env, "posted", posted, "already", skipped, "failed", failed)
	if failed > 0 {
		return fmt.Errorf("%d syndication(s) failed", failed)
	}
	return nil
}

// syndicateTargets intersects a post's chosen targets with the env's allowed set; nil chosen
// (no per-post `syndicate:` list) means all allowed targets.
func syndicateTargets(chosen, allowed []string) []string {
	if len(chosen) == 0 {
		return allowed
	}
	var out []string
	for _, id := range chosen {
		if contains(allowed, id) {
			out = append(out, id)
		}
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func stampDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
