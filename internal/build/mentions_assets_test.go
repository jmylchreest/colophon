package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmylchreest/colophon/internal/config"
	_ "github.com/jmylchreest/colophon/internal/source/mddir" // register the md-dir driver
	"github.com/jmylchreest/colophon/internal/webmention"
)

// fedFixture lays out a minimal project whose single post + author can be toggled to exercise the
// federation asset machinery: webmention display mode, a syndication frontmatter block, and an
// author profile link to a recognised silo. It returns the project root.
type fedFixture struct {
	mode        string // webmention display mode ("", "asset", "live", "disabled")
	syndication bool   // post carries a syndication: URL → renders an "Also posted on…" block
	authorSilo  bool   // author has a github.com link → author-link silo glyph
	glossary    bool   // ship a glossary term the post uses → independent engine asset
	assetData   bool   // (asset mode) seed a cached mention so mentions_html bakes
}

func writeFedFixture(t *testing.T, f fedFixture) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var wm string
	if f.mode != "" {
		wm = "\n    federation:\n      indieweb:\n        webmention:\n" +
			"          receiver: https://webmention.io/example.com/webmention\n" +
			"          source: https://webmention.io/api/mentions.jf2?domain=example.com\n" +
			"          display:\n            mode: " + f.mode + "\n"
	}
	gloss := ""
	if f.glossary {
		write("glossary.yaml", "Webmention: A way for one site to notify another that it linked to it.\n")
		gloss = "" // glossary.yaml at root is auto-discovered
	}
	_ = gloss
	write("colophon.yaml", "sites:\n  - id: main\n    title: T\n    base_url: https://example.com/\n    theme: press"+wm+"\n")

	// One post. Optionally add a syndication URL and a glossary term in the body.
	fmLines := "---\ntitle: Hello\ndate: 2026-06-20\nauthor: me\n"
	if f.syndication {
		fmLines += "syndication:\n  - https://bsky.app/profile/me/post/abc\n"
	}
	fmLines += "---\n"
	body := "Some body text."
	if f.glossary {
		body += " This mentions Webmention once."
	}
	write("content/posts/hello.md", fmLines+body)

	author := "id: me\nname: Sam Avery\n"
	if f.authorSilo {
		author += "urls:\n  - https://github.com/sam\n"
	}
	write("authors/me.yaml", author)

	if f.mode == "asset" && f.assetData {
		// Seed the received-mentions cache so asset mode has something to bake.
		key := webmention.KeyForURL("/posts/hello/")
		if err := webmention.SaveCached(webmention.CacheDir(root), key, webmention.Mentions{
			Target: "https://example.com/posts/hello/",
			Mentions: []webmention.Mention{
				{Type: "like", Author: webmention.MentionAuthor{Name: "Ada", URL: "https://bsky.app/profile/ada"}, URL: "https://bsky.app/profile/ada/post/1"},
				{Type: "reply", Author: webmention.MentionAuthor{Name: "Bo", URL: "https://hachyderm.io/@bo"}, URL: "https://hachyderm.io/@bo/2", Content: "Great post, really enjoyed the depth here.", Published: "2026-06-21T10:00:00Z"},
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func buildFed(t *testing.T, f fedFixture) string {
	t.Helper()
	root := writeFedFixture(t, f)
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "public")
	if _, err := Run(cfg, Options{OutDir: out}); err != nil {
		t.Fatal(err)
	}
	return out
}

func exists(out, rel string) bool {
	_, err := os.Stat(filepath.Join(out, filepath.FromSlash(rel)))
	return err == nil
}

// TestFederationAssetShipping is the audit: for each combination of webmention mode + syndication
// + author silos, it asserts exactly which engine assets ship and how the post page wires them —
// so nothing ships when its feature is off, and each feature works independently.
func TestFederationAssetShipping(t *testing.T) {
	post := "posts/hello/index.html"

	t.Run("disabled ships nothing", func(t *testing.T) {
		out := buildFed(t, fedFixture{mode: "disabled"})
		for _, a := range []string{"mentions.js", "mentions.css", "silos.woff2"} {
			if exists(out, a) {
				t.Errorf("disabled site shipped %s", a)
			}
		}
		html := read(t, filepath.Join(out, filepath.FromSlash(post)))
		if strings.Contains(html, "mentions.css") || strings.Contains(html, "mentions.js") {
			t.Error("disabled page links a mentions asset")
		}
		if strings.Contains(html, `class="responses"`) {
			t.Error("disabled page rendered a responses section")
		}
	})

	t.Run("no federation config ships nothing", func(t *testing.T) {
		out := buildFed(t, fedFixture{}) // mode "" → no webmention block at all
		for _, a := range []string{"mentions.js", "mentions.css", "silos.woff2"} {
			if exists(out, a) {
				t.Errorf("plain site shipped %s", a)
			}
		}
	})

	t.Run("asset mode ships js+css+font and bakes html", func(t *testing.T) {
		out := buildFed(t, fedFixture{mode: "asset", assetData: true})
		for _, a := range []string{"mentions.js", "mentions.css", "silos.woff2"} {
			if !exists(out, a) {
				t.Errorf("asset mode did not ship %s", a)
			}
		}
		// The normalised per-post asset is emitted for the JS path.
		if !exists(out, "_mentions/posts/hello.json") {
			t.Error("asset mode did not emit _mentions/posts/hello.json")
		}
		html := read(t, filepath.Join(out, filepath.FromSlash(post)))
		if !strings.Contains(html, `href="/mentions.css"`) {
			t.Error("asset page missing mentions.css link")
		}
		if !strings.Contains(html, `src="/mentions.js"`) {
			t.Error("asset page missing mentions.js script")
		}
		// Text-only bake: the rendered responses HTML is present in the page (works without JS).
		if !strings.Contains(html, `class="responses-title"`) || !strings.Contains(html, "Great post") {
			t.Error("asset mode did not bake mentions_html into the page")
		}
		// data-mentions src points at the asset (so JS refreshes the baked content).
		if !strings.Contains(html, "_mentions/posts/hello.json") {
			t.Error("asset page missing data-mentions src to the asset")
		}
	})

	t.Run("live mode ships js+css+font, no bake, no asset", func(t *testing.T) {
		out := buildFed(t, fedFixture{mode: "live"})
		for _, a := range []string{"mentions.js", "mentions.css", "silos.woff2"} {
			if !exists(out, a) {
				t.Errorf("live mode did not ship %s", a)
			}
		}
		if exists(out, "_mentions/posts/hello.json") {
			t.Error("live mode wrongly emitted a _mentions asset (browser fetches the receiver)")
		}
		html := read(t, filepath.Join(out, filepath.FromSlash(post)))
		if !strings.Contains(html, `href="/mentions.css"`) || !strings.Contains(html, `src="/mentions.js"`) {
			t.Error("live page missing mentions assets")
		}
		// No build-time mention content baked; the section carries the live receiver descriptor.
		if strings.Contains(html, `class="responses-title"`) {
			t.Error("live mode should not bake mentions_html")
		}
		if !strings.Contains(html, "data-mentions-live") {
			t.Error("live page missing data-mentions-live descriptor")
		}
	})

	t.Run("syndication without webmentions ships css+font, not js", func(t *testing.T) {
		out := buildFed(t, fedFixture{mode: "disabled", syndication: true})
		if exists(out, "mentions.js") {
			t.Error("syndication-only site wrongly shipped mentions.js")
		}
		if !exists(out, "mentions.css") {
			t.Error("syndication-only site did not ship mentions.css")
		}
		if !exists(out, "silos.woff2") {
			t.Error("syndication-only site did not ship silos.woff2")
		}
		html := read(t, filepath.Join(out, filepath.FromSlash(post)))
		if !strings.Contains(html, `href="/mentions.css"`) {
			t.Error("syndication page missing mentions.css link")
		}
		if strings.Contains(html, `src="/mentions.js"`) {
			t.Error("syndication page wrongly linked mentions.js")
		}
		if !strings.Contains(html, "post-syndication") {
			t.Error("syndication block not rendered")
		}
	})

	t.Run("author silos alone ship only the font", func(t *testing.T) {
		out := buildFed(t, fedFixture{mode: "disabled", authorSilo: true})
		if !exists(out, "silos.woff2") {
			t.Error("author-silo site did not ship silos.woff2")
		}
		if exists(out, "mentions.css") || exists(out, "mentions.js") {
			t.Error("author-silo site wrongly shipped mentions.css/js")
		}
	})

	t.Run("glossary and mentions are independent", func(t *testing.T) {
		// Mentions on, glossary on: both engine asset sets ship without interfering.
		out := buildFed(t, fedFixture{mode: "live", glossary: true})
		for _, a := range []string{"mentions.js", "mentions.css", "silos.woff2", "glossary.js", "glossary.css"} {
			if !exists(out, a) {
				t.Errorf("mentions+glossary build missing %s", a)
			}
		}
		// Mentions off, glossary on: glossary ships, mentions do not.
		out2 := buildFed(t, fedFixture{mode: "disabled", glossary: true})
		if !exists(out2, "glossary.css") {
			t.Error("glossary-only build did not ship glossary.css")
		}
		if exists(out2, "mentions.css") {
			t.Error("glossary-only build wrongly shipped mentions.css")
		}
	})
}
