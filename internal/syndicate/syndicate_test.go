package syndicate

import (
	"context"
	"testing"

	"github.com/jmylchreest/colophon/internal/core"
)

func TestLedgerRoundTrip(t *testing.T) {
	root := t.TempDir()
	l, existed, err := LoadLedger(root)
	if err != nil || existed {
		t.Fatalf("fresh ledger: existed=%v err=%v", existed, err)
	}
	if l.Has("posts/x", "mastodon") {
		t.Error("empty ledger should not Has")
	}
	l.Set("posts/x", "mastodon", Record{URL: "https://m.example/1", SyndicatedAt: "2026-06-22T00:00:00Z", Fingerprint: "abc123"})
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}

	l2, existed, err := LoadLedger(root)
	if err != nil || !existed {
		t.Fatalf("reload: existed=%v err=%v", existed, err)
	}
	if !l2.Has("posts/x", "mastodon") {
		t.Error("recorded entry missing after reload")
	}
	if urls := l2.URLs("posts/x"); len(urls) != 1 || urls[0] != "https://m.example/1" {
		t.Errorf("URLs = %v", urls)
	}
}

func TestCommandSyndicator(t *testing.T) {
	conf := core.SyndicatorConf{ID: "notify", Driver: "command", Settings: map[string]any{
		// Echo the post URL back (first stdout line = silo URL); also assert env is passed.
		"command": `test -n "$COLOPHON_POST_URL" && echo "https://silo.example/$COLOPHON_POST_KEY"`,
	}}
	s, err := Open(conf)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID() != "notify" || s.Driver() != "command" {
		t.Errorf("id/driver = %q/%q", s.ID(), s.Driver())
	}
	url, err := s.Syndicate(context.Background(), Post{Key: "posts/hello", URL: "https://b.example/posts/hello/", Title: "Hello"})
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://silo.example/posts/hello" {
		t.Errorf("silo url = %q", url)
	}
}

func TestCommandSyndicatorFireAndForget(t *testing.T) {
	s, _ := Open(core.SyndicatorConf{ID: "x", Driver: "command", Settings: map[string]any{"command": "true"}})
	url, err := s.Syndicate(context.Background(), Post{Key: "k", URL: "u"})
	if err != nil || url != "" {
		t.Errorf("fire-and-forget: url=%q err=%v", url, err)
	}
}

func TestCommandSyndicatorError(t *testing.T) {
	s, _ := Open(core.SyndicatorConf{ID: "x", Driver: "command", Settings: map[string]any{"command": "echo boom >&2; exit 3"}})
	if _, err := s.Syndicate(context.Background(), Post{Key: "k"}); err == nil {
		t.Error("expected error from failing command")
	}
}

func TestOpenUnknownDriver(t *testing.T) {
	if _, err := Open(core.SyndicatorConf{ID: "x", Driver: "nope"}); err == nil {
		t.Error("expected error for unknown driver")
	}
	if _, err := Open(core.SyndicatorConf{ID: "x", Driver: "command"}); err == nil {
		t.Error("command driver without command: should error")
	}
}
