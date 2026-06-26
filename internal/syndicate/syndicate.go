// Package syndicate implements POSSE: publish on your own site, then syndicate copies to silos
// (Mastodon, Bluesky, …) linking back. Each silo is a driver behind a uniform Syndicator
// interface; a committed ledger records what's been posted so re-runs never double-post.
package syndicate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmylchreest/colophon/internal/core"
)

// Post is the content handed to a syndicator. URL is the canonical (absolute) post URL the silo
// copy links back to.
type Post struct {
	Key       string
	Title     string
	URL       string
	Summary   string
	Text      string // syndicate_text override, else empty (driver derives from title/summary)
	Tags      []string
	Published string // RFC3339, or ""
}

// Syndicator posts a copy of a post to one silo and returns its URL (empty = fire-and-forget,
// no URL to record). Implementations must be idempotent only insofar as the ledger guards them;
// they should assume they are called once per (post, driver).
type Syndicator interface {
	ID() string
	Driver() string
	Syndicate(ctx context.Context, p Post) (siloURL string, err error)
}

// Updater is an optional Syndicator capability: edit an already-syndicated copy in place when the
// post's content changes (title/summary/text/link). A driver implements it only if its silo can
// edit a published copy — Mastodon (PUT status) and Bluesky (putRecord) do; Bridgy can't, so the
// run loop leaves its copy untouched and warns. prior is the existing ledger record (its URL is
// the handle the edit derives from).
type Updater interface {
	Update(ctx context.Context, p Post, prior Record) (siloURL string, err error)
}

// Fingerprint hashes the fields that feed a silo copy, so the ledger can tell when a syndicated
// post has changed and needs re-editing. Any edit to title, summary, custom text, link or tags
// yields a new value.
func Fingerprint(p Post) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s", p.Title, p.Summary, p.Text, p.URL, strings.Join(p.Tags, ","))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Open constructs a syndicator from its config. The driver picks the mechanic.
func Open(conf core.SyndicatorConf) (Syndicator, error) {
	switch conf.Driver {
	case "command":
		return newCommandSyndicator(conf)
	case "bluesky":
		return newBlueskySyndicator(conf)
	case "mastodon":
		return newMastodonSyndicator(conf)
	case "bridgy":
		return newBridgySyndicator(conf)
	default:
		return nil, fmt.Errorf("syndicator %q: unknown driver %q (have: command, bluesky, mastodon, bridgy)", conf.ID, conf.Driver)
	}
}

// --- ledger ---

// Ledger is the authoritative, committed record of which posts have been syndicated to which
// driver (post key → driver id → record). It is the idempotency guard: `syndicate` skips a
// (post, driver) already present, so it never re-posts. It must be committed — a fresh runner
// without it would treat every post as new.
type Ledger struct {
	path    string
	Entries map[string]map[string]Record `json:"entries"`
}

// Record is one syndication result. Fingerprint is the content hash at the time the copy was
// posted/edited; a later run compares it to decide whether the silo copy is stale. It is absent on
// ledgers written before update support — the first run after upgrade backfills it (without
// editing, since there's nothing to compare against).
type Record struct {
	URL          string `json:"url,omitempty"`
	SyndicatedAt string `json:"syndicated_at,omitempty"`
	Fingerprint  string `json:"fingerprint,omitempty"`
}

// LedgerPath is the committed ledger location under a project root.
func LedgerPath(root string) string {
	return filepath.Join(root, ".colophon", "syndication.json")
}

// LoadLedger reads the ledger; a missing file yields an empty ledger (Existed reports which).
func LoadLedger(root string) (l *Ledger, existed bool, err error) {
	path := LedgerPath(root)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Ledger{path: path, Entries: map[string]map[string]Record{}}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	l = &Ledger{path: path}
	if err := json.Unmarshal(b, l); err != nil {
		return nil, true, err
	}
	if l.Entries == nil {
		l.Entries = map[string]map[string]Record{}
	}
	l.path = path
	return l, true, nil
}

// Has reports whether a post has already been syndicated to a driver.
func (l *Ledger) Has(key, driver string) bool {
	_, ok := l.Entries[key][driver]
	return ok
}

// Get returns the recorded result for a (post, driver), and whether one exists.
func (l *Ledger) Get(key, driver string) (Record, bool) {
	r, ok := l.Entries[key][driver]
	return r, ok
}

// Set stores a syndication result (post key → driver → record).
func (l *Ledger) Set(key, driver string, rec Record) {
	if l.Entries[key] == nil {
		l.Entries[key] = map[string]Record{}
	}
	l.Entries[key][driver] = rec
}

// URLs returns the recorded silo URLs for a post (for u-syndication), order unspecified.
func (l *Ledger) URLs(key string) []string {
	var out []string
	for _, r := range l.Entries[key] {
		if r.URL != "" {
			out = append(out, r.URL)
		}
	}
	return out
}

// Save writes the ledger (encoding/json sorts map keys, so diffs stay stable).
func (l *Ledger) Save() error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(l.path, append(b, '\n'), 0o644)
}
