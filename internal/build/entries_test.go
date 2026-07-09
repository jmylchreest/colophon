package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	_ "github.com/jmylchreest/colophon/internal/source/mddir" // register the md-dir driver
)

func TestEntriesAndSlugs(t *testing.T) {
	dir := t.TempDir()
	write := func(p, body string) {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("content/posts/hello.md", "---\ntitle: Hello\ndate: 2026-01-02\nauthor: me\npersona: technical\ntags: [go]\n---\nbody")
	write("content/pages/about.md", "---\ntitle: About\n---\nbody")
	write("content/posts/withdesc.md", "---\ntitle: D\ndescription: An explicit summary.\n---\n## Heading\n\nA first paragraph of prose.")

	cfg := &config.Config{Root: dir}
	entries, err := Entries(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}
	bySlug := map[string]Entry{}
	for _, e := range entries {
		bySlug[e.Slug] = e
	}
	if h := bySlug["posts/hello"]; h.Type != "post" || h.Author != "me" || h.Persona != "technical" {
		t.Errorf("hello = %+v; want type=post author=me persona=technical", h)
	}
	// No `description:` → excerpt of the rendered body.
	if d := bySlug["posts/hello"].Description; d != "body" {
		t.Errorf("hello description fallback = %q, want body excerpt %q", d, "body")
	}
	// Explicit `description:` is kept verbatim (no excerpt, heading not leaked).
	if d := bySlug["posts/withdesc"].Description; d != "An explicit summary." {
		t.Errorf("withdesc description = %q, want the explicit value", d)
	}
	if a := bySlug["pages/about"]; a.Type != "page" { // dateless → page
		t.Errorf("about type = %q, want page", a.Type)
	}
	slugs, err := Slugs(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !slugs["posts/hello"] || !slugs["pages/about"] {
		t.Errorf("slugs = %v", slugs)
	}
}

// Entries must route translations exactly as the build does: the non-default language gets
// the /<lang>/ prefix, so doctor sees no phantom slug collision and syndication keys/permalinks
// point at the real published URL.
func TestEntriesTranslationRouting(t *testing.T) {
	dir := t.TempDir()
	write := func(p, body string) {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("content/posts/hello.md", "---\ntitle: Hello\ndate: 2026-01-02\n---\nbody")
	write("content/posts/hello.es.md", "---\ntitle: Hola\ndate: 2026-01-02\n---\ncuerpo")

	cfg := &config.Config{Root: dir, Sites: []core.Site{{Lang: "en", Languages: []string{"en", "es"}}}}
	entries, err := Entries(cfg)
	if err != nil {
		t.Fatal(err)
	}
	byURL := map[string]Entry{}
	for _, e := range entries {
		if prev, dup := byURL[e.URL]; dup {
			t.Fatalf("URL collision between %q and %q at %q", prev.Title, e.Title, e.URL)
		}
		byURL[e.URL] = e
	}
	en, ok := byURL["posts/hello/"]
	if !ok || en.Lang != "en" || en.TransKey != "posts/hello" {
		t.Errorf("default-language entry = %+v; want URL posts/hello/ lang=en transKey=posts/hello", en)
	}
	es, ok := byURL["es/posts/hello/"]
	if !ok || es.Lang != "es" || es.TransKey != "posts/hello" {
		t.Errorf("translation entry = %+v; want URL es/posts/hello/ lang=es transKey=posts/hello", es)
	}

	// Slugs reserves both the routed slug and the language-neutral base.
	slugs, err := Slugs(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !slugs["posts/hello"] || !slugs["es/posts/hello"] {
		t.Errorf("slugs = %v", slugs)
	}
}
