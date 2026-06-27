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
	Resync       bool   `help:"Re-edit every already-syndicated copy to the post's current content, ignoring fingerprints (one-shot; only drivers that can edit, e.g. mastodon/bluesky)"`
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

	posted, updated, backfilled, skipped, failed := 0, 0, 0, 0, 0
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
		fp := syndicate.Fingerprint(post)
		for _, id := range targets {
			prior, exists := ledger.Get(post.Key, id)
			switch {
			case !exists:
				// New (post, driver) → post a fresh copy.
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
				ledger.Set(post.Key, id, syndicate.Record{URL: url, SyndicatedAt: now, Fingerprint: fp})
				posted++
				log.Step("SYNDICATE", "post", "url", post.URL, "to", id, "silo", url, "status", "ok")

			case !c.Resync && prior.Fingerprint == "":
				// Ledger entry written before update support: record the current fingerprint
				// without editing (there's nothing to compare against), so future edits to the
				// post are detected from here on. This is the one-time backfill. (--resync skips
				// this and forces an edit instead.)
				if c.DryRun {
					log.Step("SYNDICATE", "would-backfill", "post", post.URL, "to", id)
					backfilled++
					continue
				}
				prior.Fingerprint = fp
				ledger.Set(post.Key, id, prior)
				backfilled++

			case !c.Resync && prior.Fingerprint == fp:
				skipped++ // unchanged since last syndicated

			default:
				// Content changed (or --resync) → bring the silo copy up to date.
				if strings.TrimSpace(prior.URL) == "" {
					// No silo handle was ever recorded (e.g. posted via a fire-and-forget driver
					// like Bridgy) → nothing to edit. Skip cleanly; this isn't a failure.
					skipped++
					log.Step("SYNDICATE", "edit", "url", post.URL, "to", id, "status", "skipped", "note", "no recorded silo URL to edit")
					continue
				}
				// In-place editors (Mastodon) edit on any change, preserving engagement. Replace-only
				// silos (Bluesky) can't edit a card visibly, so they only act on an explicit --resync,
				// where the swap's engagement reset is opted into.
				up, isUpdater := open[id].(syndicate.Updater)
				rp, isReplacer := open[id].(syndicate.Replacer)
				var action string
				var edit func() (string, error)
				switch {
				case isUpdater:
					action, edit = "edit", func() (string, error) { return up.Update(context.Background(), post, prior) }
				case isReplacer && c.Resync:
					action, edit = "replace", func() (string, error) { return rp.Replace(context.Background(), post, prior) }
				case isReplacer:
					skipped++
					log.Step("SYNDICATE", "edit", "url", post.URL, "to", id, "status", "skipped", "note", "silo can't edit a card in place; run --resync to refresh (resets likes/replies)")
					continue
				default:
					skipped++
					log.Step("SYNDICATE", "edit", "url", post.URL, "to", id, "status", "unsupported", "note", "driver can't edit a published copy")
					continue
				}
				if c.DryRun {
					log.Step("SYNDICATE", "would-"+action, "post", post.URL, "to", id)
					updated++
					continue
				}
				url, err := edit()
				if err != nil {
					failed++
					log.Step("SYNDICATE", action, "url", post.URL, "to", id, "status", "failed", "error", err.Error())
					continue
				}
				if strings.TrimSpace(url) != "" {
					prior.URL = url
				}
				prior.Fingerprint, prior.SyndicatedAt = fp, now
				ledger.Set(post.Key, id, prior)
				updated++
				log.Step("SYNDICATE", action, "url", post.URL, "to", id, "silo", prior.URL, "status", "ok")
			}
		}
	}

	if c.DryRun {
		log.Step("SYNDICATE", c.Env, "dry_run", true, "would_post", posted, "would_update", updated, "would_backfill", backfilled, "already", skipped)
		return nil
	}
	if err := ledger.Save(); err != nil {
		return fmt.Errorf("save ledger: %w", err)
	}
	log.Step("SYNDICATE", c.Env, "posted", posted, "updated", updated, "backfilled", backfilled, "already", skipped, "failed", failed)
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
