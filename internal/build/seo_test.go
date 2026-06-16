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
