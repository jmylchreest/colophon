// Package syndicate implements POSSE: publish on your own site, then syndicate copies to silos
// (Mastodon, Bluesky, …) linking back. Each silo is a driver behind a uniform Syndicator
// interface; a committed ledger records what's been posted so re-runs never double-post.
package syndicate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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

// Open constructs a syndicator from its config. The driver picks the mechanic.
func Open(conf core.SyndicatorConf) (Syndicator, error) {
	switch conf.Driver {
	case "command":
		return newCommandSyndicator(conf)
	default:
		return nil, fmt.Errorf("syndicator %q: unknown driver %q (have: command)", conf.ID, conf.Driver)
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

// Record is one syndication result.
type Record struct {
	URL          string `json:"url,omitempty"`
	SyndicatedAt string `json:"syndicated_at,omitempty"`
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

// Record stores a syndication result.
func (l *Ledger) Set(key, driver, url, at string) {
	if l.Entries[key] == nil {
		l.Entries[key] = map[string]Record{}
	}
	l.Entries[key][driver] = Record{URL: url, SyndicatedAt: at}
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
