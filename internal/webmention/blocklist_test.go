package webmention

import (
	"os"
	"path/filepath"
	"testing"
)

func writeBlock(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".colophon"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(BlocklistPath(root), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestBlocklist(t *testing.T) {
	root := writeBlock(t, `
- "*.spam.example"
- author.url: "https://troll.example/*"
- content: "*free crypto*"
- type: "bookmark"
`)
	bl, err := LoadBlocklist(root)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		m    Mention
		want bool
	}{
		{"domain glob via author host", Mention{Author: MentionAuthor{URL: "https://bad.spam.example/x"}}, true},
		{"author.url glob", Mention{Author: MentionAuthor{URL: "https://troll.example/abc"}}, true},
		{"content substring", Mention{Content: "get free crypto now"}, true},
		{"type", Mention{Type: "bookmark"}, true},
		{"clean reply", Mention{Type: "reply", Author: MentionAuthor{URL: "https://nice.example"}, Content: "hi"}, false},
	}
	for _, c := range cases {
		if got := bl.Match(c.m); got != c.want {
			t.Errorf("%s: Match = %v, want %v", c.name, got, c.want)
		}
	}

	kept := bl.Filter([]Mention{
		{Type: "reply", Content: "free crypto!!"},
		{Type: "reply", Author: MentionAuthor{URL: "https://ok.example"}, Content: "nice"},
	})
	if len(kept) != 1 || kept[0].Content != "nice" {
		t.Errorf("Filter kept %+v", kept)
	}
	if pats := bl.ClientPatterns(); len(pats) != 4 {
		t.Errorf("ClientPatterns = %v", pats)
	}
}

func TestBlocklistMissingIsEmpty(t *testing.T) {
	bl, err := LoadBlocklist(t.TempDir())
	if err != nil || !bl.Empty() {
		t.Fatalf("missing blocklist should be empty, no error; got %v err=%v", bl, err)
	}
	// Empty list is a pass-through.
	ms := []Mention{{Type: "reply"}}
	if got := bl.Filter(ms); len(got) != 1 {
		t.Errorf("empty blocklist filtered %d", len(got))
	}
}
