package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmylchreest/colophon/internal/build"
	"github.com/jmylchreest/colophon/internal/clog"
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
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

	open, err := openSyndicators(site.Federation.Syndication, allowed, c.Env)
	if err != nil {
		return err
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

	r := &synRunner{open: open, ledger: ledger, log: log, dryRun: c.DryRun, resync: c.Resync, now: now}
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
			r.reconcile(post, fp, id)
		}
	}

	if c.DryRun {
		log.Step("SYNDICATE", c.Env, "dry_run", true, "would_post", r.posted, "would_update", r.updated, "would_backfill", r.backfilled, "already", r.skipped)
		return nil
	}
	if err := ledger.Save(); err != nil {
		return fmt.Errorf("save ledger: %w", err)
	}
	log.Step("SYNDICATE", c.Env, "posted", r.posted, "updated", r.updated, "backfilled", r.backfilled, "already", r.skipped, "failed", r.failed)
	if r.failed > 0 {
		return fmt.Errorf("%d syndication(s) failed", r.failed)
	}
	return nil
}

// openSyndicators opens the configured syndicators named by the env's allowed set (and only
// those), erroring when the env names an id with no federation.syndication entry.
func openSyndicators(confs []core.SyndicatorConf, allowed []string, envName string) (map[string]syndicate.Syndicator, error) {
	open := map[string]syndicate.Syndicator{}
	for _, sc := range confs {
		if !contains(allowed, sc.ID) {
			continue
		}
		s, err := syndicate.Open(sc)
		if err != nil {
			return nil, err
		}
		open[sc.ID] = s
	}
	for _, id := range allowed {
		if _, ok := open[id]; !ok {
			return nil, fmt.Errorf("environment %q syndicates to %q, but no federation.syndication entry has that id", envName, id)
		}
	}
	return open, nil
}

// synRunner reconciles each eligible (post, target) pair against the ledger, one method per
// ledger state, counting what happened (or, in dry-run, what would). It mutates the ledger but
// never saves it — the caller decides (dry-run discards).
type synRunner struct {
	open   map[string]syndicate.Syndicator
	ledger *syndicate.Ledger
	log    *clog.Logger
	dryRun bool
	resync bool
	now    string // RFC3339 stamp written to touched ledger records

	posted, updated, backfilled, skipped, failed int
}

// reconcile routes one (post, target) pair to the handler for its ledger state.
func (r *synRunner) reconcile(post syndicate.Post, fp, id string) {
	prior, exists := r.ledger.Get(post.Key, id)
	switch {
	case !exists:
		r.postNew(post, fp, id)
	case !r.resync && prior.Fingerprint == "":
		r.backfill(post, fp, id, prior)
	case !r.resync && prior.Fingerprint == fp:
		r.skipped++ // unchanged since last syndicated
	default:
		r.edit(post, fp, id, prior)
	}
}

// postNew posts a fresh copy for a (post, driver) pair the ledger has never seen.
func (r *synRunner) postNew(post syndicate.Post, fp, id string) {
	if r.dryRun {
		r.log.Step("SYNDICATE", "would-post", "post", post.URL, "to", id)
		r.posted++
		return
	}
	url, err := r.open[id].Syndicate(context.Background(), post)
	if err != nil {
		r.failed++
		r.log.Step("SYNDICATE", "post", "url", post.URL, "to", id, "status", "failed", "error", err.Error())
		return
	}
	r.ledger.Set(post.Key, id, syndicate.Record{URL: url, SyndicatedAt: r.now, Fingerprint: fp})
	r.posted++
	r.log.Step("SYNDICATE", "post", "url", post.URL, "to", id, "silo", url, "status", "ok")
}

// backfill handles a ledger entry written before update support: record the current
// fingerprint without editing (there's nothing to compare against), so future edits to the
// post are detected from here on. This is the one-time backfill. (--resync skips this and
// forces an edit instead.)
func (r *synRunner) backfill(post syndicate.Post, fp, id string, prior syndicate.Record) {
	if r.dryRun {
		r.log.Step("SYNDICATE", "would-backfill", "post", post.URL, "to", id)
		r.backfilled++
		return
	}
	prior.Fingerprint = fp
	r.ledger.Set(post.Key, id, prior)
	r.backfilled++
}

// edit brings a changed (or --resync'd) silo copy up to date, via whichever capability the
// driver has. In-place editors (Mastodon, syndicate.Updater) edit on any change, preserving
// engagement. Replace-only silos (Bluesky, syndicate.Replacer) can't edit a card visibly, so
// they only act on an explicit --resync, where the swap's engagement reset is opted into.
func (r *synRunner) edit(post syndicate.Post, fp, id string, prior syndicate.Record) {
	if strings.TrimSpace(prior.URL) == "" {
		// No silo handle was ever recorded (e.g. posted via a fire-and-forget driver like
		// Bridgy) → nothing to edit. Skip cleanly; this isn't a failure.
		r.skipped++
		r.log.Step("SYNDICATE", "edit", "url", post.URL, "to", id, "status", "skipped", "note", "no recorded silo URL to edit")
		return
	}
	up, isUpdater := r.open[id].(syndicate.Updater)
	rp, isReplacer := r.open[id].(syndicate.Replacer)
	var action string
	var edit func() (string, error)
	switch {
	case isUpdater:
		action, edit = "edit", func() (string, error) { return up.Update(context.Background(), post, prior) }
	case isReplacer && r.resync:
		action, edit = "replace", func() (string, error) { return rp.Replace(context.Background(), post, prior) }
	case isReplacer:
		r.skipped++
		r.log.Step("SYNDICATE", "edit", "url", post.URL, "to", id, "status", "skipped", "note", "silo can't edit a card in place; run --resync to refresh (resets likes/replies)")
		return
	default:
		r.skipped++
		r.log.Step("SYNDICATE", "edit", "url", post.URL, "to", id, "status", "unsupported", "note", "driver can't edit a published copy")
		return
	}
	if r.dryRun {
		r.log.Step("SYNDICATE", "would-"+action, "post", post.URL, "to", id)
		r.updated++
		return
	}
	url, err := edit()
	if err != nil {
		r.failed++
		r.log.Step("SYNDICATE", action, "url", post.URL, "to", id, "status", "failed", "error", err.Error())
		return
	}
	if strings.TrimSpace(url) != "" {
		prior.URL = url
	}
	prior.Fingerprint, prior.SyndicatedAt = fp, r.now
	r.ledger.Set(post.Key, id, prior)
	r.updated++
	r.log.Step("SYNDICATE", action, "url", post.URL, "to", id, "silo", prior.URL, "status", "ok")
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
