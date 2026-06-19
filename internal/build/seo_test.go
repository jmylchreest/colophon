package build

import (
	"strings"
	"testing"
	"time"

	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/markdown"
)

func TestSEOHeadDefaults(t *testing.T) {
	site := core.Site{Title: "My Blog", BaseURL: "https://example.com"}
	author := core.Author{ID: "ada", Name: "Ada", URLs: []string{"https://ada.example"}}
	p := page{
		Title: "Hello", Description: "A greeting", URL: "posts/hello/",
		Published: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Tags:      []string{"go"}, ImageAbs: "https://example.com/posts/hello/og.png",
	}
	out := seoHead(site, p, author)
	for _, want := range []string{
		`<link rel="canonical" href="https://example.com/posts/hello/">`,
		`<meta property="og:type" content="article">`,
		`<meta property="og:title" content="Hello">`,
		`<meta property="og:image" content="https://example.com/posts/hello/og.png">`,
		`<meta property="article:tag" content="go">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`"@type":"BlogPosting"`,
		`"author":{"@type":"Person","name":"Ada","url":"https://ada.example"}`,
		`"publisher":{"@type":"Organization","name":"My Blog"}`,
		`"datePublished":"2026-01-02T00:00:00Z"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("seoHead missing %s\n%s", want, out)
		}
	}
	if strings.Contains(out, "noindex") {
		t.Error("a published post should not be noindex")
	}
}

func TestSEOOverridesAndNoindex(t *testing.T) {
	site := core.Site{Title: "B", BaseURL: "https://e.com"}
	p := page{
		Title: "T", Description: "D", URL: "x/", Draft: true,
		SEO: &markdown.SEO{
			Title: "SEO Title", Canonical: "https://canon.example/x",
			Type: "Article", Keywords: []string{"a", "b"},
			Social: &markdown.SEOSocial{Title: "Punchy social"},
		},
	}
	out := seoHead(site, p, core.Author{})
	for _, want := range []string{
		`<meta property="og:title" content="Punchy social">`, // social title wins for OG
		`href="https://canon.example/x"`,                     // canonical override
		`"@type":"Article"`,                                  // schema type override
		`<meta name="keywords" content="a, b">`,
		`content="noindex,nofollow"`, // draft → noindex
	} {
		if !strings.Contains(out, want) {
			t.Errorf("seoHead missing %s\n%s", want, out)
		}
	}
	// No image → summary card, not large.
	if !strings.Contains(out, `<meta name="twitter:card" content="summary">`) {
		t.Error("expected summary card when no image")
	}
	// <title> uses the seo.title, not the social title.
	if metaTitle(p) != "SEO Title" {
		t.Errorf("metaTitle = %q", metaTitle(p))
	}
}

func TestSEOAuthorIsPerson(t *testing.T) {
	out := seoHead(core.Site{Title: "B", BaseURL: "https://e.com"}, page{Title: "T", URL: "x/"},
		core.Author{ID: "acme", Name: "Acme"})
	if !strings.Contains(out, `"author":{"@type":"Person","name":"Acme"}`) {
		t.Errorf("author should map to a Person\n%s", out)
	}
}

// TestSEOHeroFallback asserts a post with only a hero (no image:) still gets og:image,
// twitter:image and a JSON-LD image from the hero, and that og:locale comes from the lang.
func TestSEOHeroFallback(t *testing.T) {
	site := core.Site{Title: "B", BaseURL: "https://e.com", Lang: "en-GB"}
	p := page{Title: "T", URL: "p/", HeroAbs: "https://e.com/p/cover.png"}
	out := seoHead(site, p, core.Author{})
	for _, want := range []string{
		`<meta property="og:image" content="https://e.com/p/cover.png">`,
		`<meta name="twitter:image" content="https://e.com/p/cover.png">`,
		`<meta name="twitter:card" content="summary_large_image">`,
		`<meta property="og:locale" content="en_GB">`,
		`"image":"https://e.com/p/cover.png"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("seoHead missing %s\n%s", want, out)
		}
	}
}

// TestListingSEOHead asserts the home page and the narrower listings get the right canonical,
// website Open Graph, og:locale, and the Blog vs CollectionPage JSON-LD.
func TestListingSEOHead(t *testing.T) {
	site := core.Site{Title: "My Blog", BaseURL: "https://example.com", Lang: "en"}

	home := listingSEOHead(site, "https://example.com/", "My Blog", "A blog.", "https://example.com/og.png", true)
	for _, want := range []string{
		`<link rel="canonical" href="https://example.com/">`,
		`<meta property="og:type" content="website">`,
		`<meta property="og:description" content="A blog.">`,
		`<meta property="og:image" content="https://example.com/og.png">`,
		`<meta property="og:locale" content="en">`,
		`"@type":"Blog"`,
	} {
		if !strings.Contains(home, want) {
			t.Errorf("home listing head missing %s\n%s", want, home)
		}
	}

	tag := listingSEOHead(site, "https://example.com/tags/go/", "Tagged go", "A blog.", "", false)
	if !strings.Contains(tag, `"@type":"CollectionPage"`) {
		t.Errorf("tag listing should be a CollectionPage\n%s", tag)
	}
	if !strings.Contains(tag, `<meta name="twitter:card" content="summary">`) {
		t.Errorf("no image → summary card\n%s", tag)
	}
}
