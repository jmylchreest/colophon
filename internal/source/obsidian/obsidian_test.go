package obsidian

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
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

	src, err := New("/", config.SourceConfig{ID: "v", Driver: "obsidian", Settings: map[string]any{"vault": dir}})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := src.Documents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := docPaths(docs); len(got) != 2 || got[0] != "a.md" || got[1] != "sub/b.md" {
		t.Errorf("publish-required vault = %v, want [a.md sub/b.md]", got)
	}
}

func TestPublishNotRequired(t *testing.T) {
	dir := writeVault(t)
	src, err := New("/", config.SourceConfig{ID: "v", Driver: "obsidian",
		Settings: map[string]any{"vault": dir, "publish_required": false}})
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
	src, err := New("/", config.SourceConfig{ID: "v", Driver: "obsidian", Settings: map[string]any{"vault": dir}})
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

// writeTree writes name->content files under a fresh temp dir and returns it.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
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

func docPaths(docs []core.Content) []string {
	var got []string
	for _, d := range docs {
		got = append(got, d.SourcePath)
	}
	sort.Strings(got)
	return got
}

func TestTagModeDiscovery(t *testing.T) {
	vault := writeTree(t, map[string]string{
		// frontmatter tag
		"a.md": "---\ntitle: A\ntags: [blog, go]\n---\nbody a",
		// inline tag, nested under the configured tag
		"notes/b.md": "---\ntitle: B\n---\nbody b\n\n#blog/published here",
		// untagged → excluded
		"c.md": "---\ntitle: C\ntags: [draft]\n---\nbody c",
		// tagged but explicitly opted out
		"d.md": "---\ntitle: D\ntags: [blog]\npublish: false\n---\nbody d",
		// '#blog' inside a code fence must NOT count as a tag
		"e.md": "---\ntitle: E\n---\nbody e\n\n```\n#blog not a tag\n```",
	})
	src, err := New("/", config.SourceConfig{ID: "v", Driver: "obsidian",
		Settings: map[string]any{"vault": vault, "tag": "blog"}})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := src.Documents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := docPaths(docs)
	if len(got) != 2 || got[0] != "a.md" || got[1] != "notes/b.md" {
		t.Errorf("tag-mode discovery = %v, want [a.md notes/b.md]", got)
	}
}

// Selection is the source's only filtering concern: a tag-matched note is returned even if
// it is malformed (no title, empty body). Whether it is well-formed is the build's concern,
// so the source neither skips nor warns about format.
func TestTagSelectionIgnoresFormat(t *testing.T) {
	vault := writeTree(t, map[string]string{
		"ok.md":      "---\ntitle: Good\ntags: [blog]\n---\nhas a title and body",
		"notitle.md": "no title or heading here, just body\n\n#blog",
		"empty.md":   "---\ntitle: Empty\ntags: [blog]\n---\n",
	})
	src, err := New("/", config.SourceConfig{ID: "v", Driver: "obsidian",
		Settings: map[string]any{"vault": vault, "tag": "blog"}})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := src.Documents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := docPaths(docs); len(got) != 3 {
		t.Errorf("all 3 tagged notes should be selected regardless of format, got %v", got)
	}
	if w := src.(core.Warner).Warnings(); len(w) != 0 {
		t.Errorf("source should not warn about format, got %v", w)
	}
}

func TestVaultWithBlogSubfolderResolvesVaultWideEmbeds(t *testing.T) {
	vault := writeTree(t, map[string]string{
		"assets/pic.png":   "img",
		"blog/post.md":     "---\ntitle: P\npublish: true\n---\n![[pic.png]]",
		"blog/sub/deep.md": "---\ntitle: D\npublish: true\n---\n![[pic.png]]",
	})
	// A sub-folder of the vault; attachments live elsewhere in the vault and must still resolve.
	src, err := New("/", config.SourceConfig{ID: "v", Driver: "obsidian",
		Settings: map[string]any{"vault": vault, "path": "blog"}})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := src.Documents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]string{}
	for _, d := range docs {
		body[d.SourcePath] = d.Body
	}
	if got := body["post.md"]; !strings.Contains(got, "![](<../assets/pic.png>)") {
		t.Errorf("post.md embed should reach the vault-level asset: %q", got)
	}
	if got := body["sub/deep.md"]; !strings.Contains(got, "![](<../../assets/pic.png>)") {
		t.Errorf("sub/deep.md embed should reach the vault-level asset: %q", got)
	}
}

func TestVaultOnlyScansWholeVault(t *testing.T) {
	vault := writeTree(t, map[string]string{
		"a.md":      "---\ntitle: A\npublish: true\n---\nx",
		"deep/b.md": "---\ntitle: B\n---\nno flag",
	})
	src, err := New("/", config.SourceConfig{ID: "v", Driver: "obsidian",
		Settings: map[string]any{"vault": vault}}) // no path, no tag → whole vault + publish gate
	if err != nil {
		t.Fatal(err)
	}
	docs, err := src.Documents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := docPaths(docs); len(got) != 1 || got[0] != "a.md" {
		t.Errorf("vault-only with publish gate = %v, want [a.md]", got)
	}
}

func TestMultiplePathsAndTagsAreOredWithinAndAndedAcross(t *testing.T) {
	vault := writeTree(t, map[string]string{
		"essays/x.md": "---\ntitle: X\ntags: [blog]\n---\nin essays, tagged blog",
		"essays/y.md": "---\ntitle: Y\ntags: [essay]\n---\nin essays, tagged essay",
		"essays/z.md": "---\ntitle: Z\ntags: [draft]\n---\nin essays, untagged-for-us",
		"til/t.md":    "---\ntitle: T\ntags: [essay]\n---\nin til, tagged essay",
		"other/o.md":  "---\ntitle: O\ntags: [blog]\n---\noutside the scanned paths",
	})
	src, err := New("/", config.SourceConfig{ID: "v", Driver: "obsidian", Settings: map[string]any{
		"vault": vault,
		"path":  []any{"essays", "til"}, // union of paths
		"tag":   []any{"blog", "essay"}, // a note matches any tag
	}})
	if err != nil {
		t.Fatal(err)
	}
	docs, err := src.Documents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Under essays|til AND tagged blog|essay: x (blog), y (essay), t (essay). z is untagged;
	// o is tagged but outside the scanned paths. SourcePath is relative to each scanned root,
	// so slugs stay flat (Obsidian note names are unique vault-wide).
	if got := docPaths(docs); len(got) != 3 || got[0] != "t.md" || got[1] != "x.md" || got[2] != "y.md" {
		t.Errorf("multi path×tag = %v, want [t.md x.md y.md]", got)
	}
}

func TestTagAndPathAcceptScalarOrList(t *testing.T) {
	files := map[string]string{"blog/a.md": "---\ntitle: A\ntags: [blog]\n---\nx"}
	vault := writeTree(t, files)
	scalar, _ := New("/", config.SourceConfig{ID: "v", Driver: "obsidian",
		Settings: map[string]any{"vault": vault, "path": "blog", "tag": "blog"}})
	list, _ := New("/", config.SourceConfig{ID: "v", Driver: "obsidian",
		Settings: map[string]any{"vault": vault, "path": []any{"blog"}, "tag": []any{"blog"}}})
	ds, _ := scalar.Documents(context.Background())
	dl, _ := list.Documents(context.Background())
	if len(ds) != 1 || len(dl) != 1 {
		t.Errorf("scalar and list forms should both select a.md, got %d and %d", len(ds), len(dl))
	}
}

func TestStrList(t *testing.T) {
	cases := []struct {
		in   any
		want []string
	}{
		{"blog", []string{"blog"}},
		{"blog,essay", []string{"blog", "essay"}},     // a single env value splits on commas
		{" blog , essay ", []string{"blog", "essay"}}, // trimmed
		{[]any{"a", "b,c"}, []string{"a", "b", "c"}},  // list whose element is itself CSV
		{"", nil},
	}
	for _, c := range cases {
		got := strList(c.in)
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("strList(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// A path is vault-relative; a leading "/" (vault-root style) is tolerated, and a
// comma-separated env value yields multiple scan paths.
func TestPathLeadingSlashAndCSV(t *testing.T) {
	vault := writeTree(t, map[string]string{
		"Blog/a.md":  "---\ntitle: A\npublish: true\n---\nx",
		"Notes/b.md": "---\ntitle: B\npublish: true\n---\nx",
		"Other/c.md": "---\ntitle: C\npublish: true\n---\nx",
	})
	src, _ := New("/", config.SourceConfig{ID: "v", Driver: "obsidian",
		Settings: map[string]any{"vault": vault, "path": "/Blog,Notes"}}) // leading slash + CSV
	docs, err := src.Documents(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := docPaths(docs); len(got) != 2 || got[0] != "a.md" || got[1] != "b.md" {
		t.Errorf("path '/Blog,Notes' = %v, want [a.md b.md]", got)
	}
}
