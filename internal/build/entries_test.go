package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/colophon/internal/config"
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
