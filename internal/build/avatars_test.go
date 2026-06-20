package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	_ "github.com/jmylchreest/colophon/internal/source/mddir" // register the md-dir driver
)

// missingKinds returns the {Kind: Ref} pairs MissingAssets reports for the project at root, so a
// test can assert on the set without depending on slice order.
func missingKinds(t *testing.T, root string) map[string]string {
	t.Helper()
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	missing, err := MissingAssets(cfg)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, m := range missing {
		got[m.Kind] = m.Ref
	}
	return got
}

// avatarPNG is an 8x8 PNG — a self-contained byte the build can publish and route.
var avatarPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x08, 0x08, 0x06, 0x00, 0x00, 0x00, 0xc4, 0x0f, 0xbe,
	0x8b, 0x00, 0x00, 0x00, 0x12, 0x49, 0x44, 0x41, 0x54, 0x18, 0x57, 0x63, 0x60, 0x18, 0x05, 0xa3,
	0x60, 0x14, 0x8c, 0x02, 0x00, 0x00, 0x00, 0x04, 0x00, 0x01, 0x27, 0x34, 0x27, 0x0a, 0x00, 0x00,
	0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

// writeAvatarFixture lays out a minimal project (config, one post, an avatar file under the
// content source) and returns its root. The author's avatar is the caller's value, so a test
// can exercise the file-path, data:, http and missing cases.
func writeAvatarFixture(t *testing.T, avatar string) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel string, b []byte) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("colophon.yaml", []byte("sites:\n  - id: main\n    title: T\n    base_url: https://example.com/\n    theme: press\n"))
	write("content/posts/hello.md", []byte("---\ntitle: Hello\ndate: 2026-01-02\nauthor: me\n---\nbody text here"))
	// A second author (with a post) so the home/author author-strip renders — press hides it
	// for a single author, and these tests assert the avatar appears in that strip.
	write("content/posts/world.md", []byte("---\ntitle: World\ndate: 2026-01-01\nauthor: you\n---\nmore body text"))
	write("content/assets/face.png", avatarPNG)
	write("authors/me.yaml", []byte("id: me\nname: Sam Avery\navatar: "+avatar+"\n"))
	write("authors/you.yaml", []byte("id: you\nname: Min Park\n"))
	return root
}

func buildTo(t *testing.T, root string, opts Options) string {
	t.Helper()
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "public")
	opts.OutDir = out
	if _, err := Run(cfg, opts); err != nil {
		t.Fatal(err)
	}
	return out
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestAvatarFilePathPublished asserts a file-path avatar lands once at public/assets/<name>
// and is emitted depth-independently (the same root-anchored src at every page depth).
func TestAvatarFilePathPublished(t *testing.T) {
	root := writeAvatarFixture(t, "assets/face.png")
	out := buildTo(t, root, Options{})

	// The byte is published once at the site root, not co-located per page.
	got, err := os.ReadFile(filepath.Join(out, "assets", "face.png"))
	if err != nil {
		t.Fatalf("avatar not published to public/assets/face.png: %v", err)
	}
	if len(got) != len(avatarPNG) {
		t.Errorf("published avatar is %d bytes, want %d", len(got), len(avatarPNG))
	}

	// Depth-independent: basePath is "/" here, so every page carries src="/assets/face.png".
	const want = `src="/assets/face.png"`
	for _, page := range []string{"index.html", "posts/hello/index.html", "authors/me/index.html"} {
		html := read(t, filepath.Join(out, filepath.FromSlash(page)))
		if !strings.Contains(html, want) {
			t.Errorf("%s missing depth-independent avatar src %q", page, want)
		}
	}
}

// TestAvatarSubPath asserts a serve-style sub-path preview (basePath /main/dist/) prefixes the
// avatar src with the base path, so it still resolves from every page depth.
func TestAvatarSubPath(t *testing.T) {
	root := writeAvatarFixture(t, "assets/face.png")
	out := buildTo(t, root, Options{BasePath: "/main/dist/"})

	const want = `src="/main/dist/assets/face.png"`
	for _, page := range []string{"index.html", "posts/hello/index.html", "authors/me/index.html"} {
		html := read(t, filepath.Join(out, filepath.FromSlash(page)))
		if !strings.Contains(html, want) {
			t.Errorf("%s missing base-path-prefixed avatar src %q", page, want)
		}
	}
}

// TestAvatarRouted asserts that under an active assets route the avatar src becomes the
// object-store URL and the byte is still published for upload (matching markdown images).
func TestAvatarRouted(t *testing.T) {
	root := writeAvatarFixture(t, "assets/face.png")
	out := buildTo(t, root, Options{
		Publishers: []string{"r2"},
		Routes:     []core.RouteRule{{Match: "**assets/**", Publisher: "r2", BaseURL: "https://assets.example.com"}},
	})

	if _, err := os.ReadFile(filepath.Join(out, "assets", "face.png")); err != nil {
		t.Fatalf("routed avatar byte not published for upload: %v", err)
	}
	const want = `src="https://assets.example.com/assets/face.png"`
	html := read(t, filepath.Join(out, "index.html"))
	if !strings.Contains(html, want) {
		t.Errorf("routed avatar src not rewritten to the object store; want %q", want)
	}
}

// TestAvatarPassthrough asserts data: and http(s):// avatars are emitted verbatim and nothing
// spurious is published.
func TestAvatarPassthrough(t *testing.T) {
	cases := []string{
		`"data:image/png;base64,iVBORw0KGgo="`,
		`https://cdn.example.com/sam.png`,
	}
	for _, avatar := range cases {
		root := writeAvatarFixture(t, avatar)
		out := buildTo(t, root, Options{})
		html := read(t, filepath.Join(out, "index.html"))
		bare := strings.Trim(avatar, `"`)
		if !strings.Contains(html, bare) {
			t.Errorf("passthrough avatar %q not emitted verbatim", bare)
		}
		if _, err := os.Stat(filepath.Join(out, "assets", "face.png")); err == nil {
			t.Errorf("passthrough avatar %q wrongly published an asset", bare)
		}
	}
}

// TestAvatarMissingWarns asserts a file-path avatar no source can open is dropped (no broken
// src) rather than emitted, and nothing is published.
func TestAvatarMissingWarns(t *testing.T) {
	root := writeAvatarFixture(t, "assets/does-not-exist.png")
	out := buildTo(t, root, Options{})
	html := read(t, filepath.Join(out, "index.html"))
	if strings.Contains(html, "does-not-exist.png") {
		t.Error("missing avatar should be dropped, not emitted as a broken src")
	}
	if _, err := os.Stat(filepath.Join(out, "assets", "does-not-exist.png")); err == nil {
		t.Error("missing avatar should not publish a file")
	}
}

// TestAvatarRootFallback asserts a project-level avatar that lives at the project root (not under
// any content source) still resolves via the sources-then-root fallback, so the same `avatar:`
// value is portable across drivers. The byte sits at <root>/assets/rootface.png — the md-dir
// source (rooted at content/) can't see it; only the project-root fallback can.
func TestAvatarRootFallback(t *testing.T) {
	root := writeAvatarFixture(t, "assets/rootface.png")
	full := filepath.Join(root, "assets", "rootface.png")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, avatarPNG, 0o644); err != nil {
		t.Fatal(err)
	}

	out := buildTo(t, root, Options{})

	if _, err := os.ReadFile(filepath.Join(out, "assets", "rootface.png")); err != nil {
		t.Fatalf("root-fallback avatar not published to public/assets/rootface.png: %v", err)
	}
	const want = `src="/assets/rootface.png"`
	html := read(t, filepath.Join(out, "index.html"))
	if !strings.Contains(html, want) {
		t.Errorf("root-fallback avatar src not emitted; want %q", want)
	}
}

// TestMissingAssets covers the dry-resolve doctor uses: a resolvable avatar reports nothing, a
// data:/http(s) avatar is skipped (passthrough, never "missing"), and a broken file-path avatar
// is reported as a missing "avatar" ref.
func TestMissingAssets(t *testing.T) {
	// Resolvable: the avatar lives under the content source.
	if got := missingKinds(t, writeAvatarFixture(t, "assets/face.png")); len(got) != 0 {
		t.Errorf("resolvable avatar should report nothing missing, got %v", got)
	}

	// Passthrough: data:/http(s) avatars are never local refs, so never "missing".
	for _, avatar := range []string{`"data:image/png;base64,iVBORw0KGgo="`, `https://cdn.example.com/sam.png`} {
		if got := missingKinds(t, writeAvatarFixture(t, avatar)); len(got) != 0 {
			t.Errorf("passthrough avatar %q should report nothing missing, got %v", avatar, got)
		}
	}

	// Broken: a file-path avatar nothing can source is reported.
	got := missingKinds(t, writeAvatarFixture(t, "assets/nope.png"))
	if got["avatar"] != "assets/nope.png" {
		t.Errorf("broken avatar not reported; got %v", got)
	}
}
