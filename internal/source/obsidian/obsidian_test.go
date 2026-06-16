package obsidian

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jmylchreest/colophon/internal/config"
)

func writeVault(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"a.md":     "---\ntitle: A\npublish: true\n---\nx",
		"sub/b.md": "---\ntitle: B\npublish: true\n---\nx",
		"c.md":     "---\ntitle: C\n---\nx",                 // no flag
		"d.md":     "---\ntitle: D\npublish: false\n---\nx", // explicit no
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestPublishFilter(t *testing.T) {
	dir := writeVault(t)

	src, err := New("/", config.SourceConfig{ID: "v", Driver: "obsidian", Settings: map[string]any{"path": dir}})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := src.Documents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range docs {
		got = append(got, d.SourcePath)
	}
	sort.Strings(got)
	if len(got) != 2 || got[0] != "a.md" || got[1] != "sub/b.md" {
		t.Errorf("publish-required vault = %v, want [a.md sub/b.md]", got)
	}
}

func TestPublishNotRequired(t *testing.T) {
	dir := writeVault(t)
	src, err := New("/", config.SourceConfig{ID: "v", Driver: "obsidian",
		Settings: map[string]any{"path": dir, "publish_required": false}})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := src.Documents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 4 {
		t.Errorf("publish_required=false should include all 4, got %d", len(docs))
	}
}

func TestResolveEmbeds(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"assets/cat.png":    "img",
		"posts/note.md":     "---\ntitle: N\npublish: true\nhero: \"[[cat.png]]\"\n---\nBody ![[cat.png]] and ![[cat.png|a cat]] and ![[missing.png]]",
		"posts/sub/deep.md": "---\ntitle: D\npublish: true\n---\n![[cat.png]]",
	}
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src, err := New("/", config.SourceConfig{ID: "v", Driver: "obsidian", Settings: map[string]any{"path": dir}})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := src.Documents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]string{}
	hero := map[string]string{}
	for _, d := range docs {
		byPath[d.SourcePath] = d.Body
		hero[d.SourcePath] = d.Frontmatter.Hero
	}

	note := byPath["posts/note.md"]
	if !strings.Contains(note, "![](<../assets/cat.png>)") {
		t.Errorf("embed not resolved note-relative: %q", note)
	}
	if !strings.Contains(note, "![a cat](<../assets/cat.png>)") {
		t.Errorf("aliased embed not resolved: %q", note)
	}
	if !strings.Contains(note, "![[missing.png]]") {
		t.Errorf("unresolved embed should be left untouched: %q", note)
	}
	if hero["posts/note.md"] != "../assets/cat.png" {
		t.Errorf("hero frontmatter not resolved: %q", hero["posts/note.md"])
	}
	if deep := byPath["posts/sub/deep.md"]; !strings.Contains(deep, "![](<../../assets/cat.png>)") {
		t.Errorf("deep note embed not resolved: %q", deep)
	}
}

func TestTitleFromBody(t *testing.T) {
	tests := []struct {
		body, src, wantTitle, wantBody string
	}{
		{"# My Title\n\nthe body", "notes/x.md", "My Title", "the body"},
		{"no heading\nmore", "notes/My Great Note.md", "My Great Note", "no heading\nmore"},
		{"# Only", "a.md", "Only", ""},
		{"  \n# Spaced\ntext", "a.md", "Spaced", "text"},
	}
	for _, tt := range tests {
		gotTitle, gotBody := titleFromBody(tt.body, tt.src)
		if gotTitle != tt.wantTitle || gotBody != tt.wantBody {
			t.Errorf("titleFromBody(%q, %q) = (%q, %q), want (%q, %q)",
				tt.body, tt.src, gotTitle, gotBody, tt.wantTitle, tt.wantBody)
		}
	}
}

func TestEmptyPathYieldsNothing(t *testing.T) {
	src, err := New("/", config.SourceConfig{ID: "v", Driver: "obsidian"})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := src.Documents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 0 {
		t.Errorf("empty-path source should yield no documents, got %d", len(docs))
	}
}
