package build

import (
	"strings"
	"testing"
	"time"

	"github.com/jmylchreest/colophon/internal/render"
	"github.com/jmylchreest/colophon/markdown"
)

func TestResolvePageType(t *testing.T) {
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		fm   markdown.Frontmatter
		want string
	}{
		{markdown.Frontmatter{Date: jan1}, "post"},               // dated → post
		{markdown.Frontmatter{}, "page"},                         // dateless → page
		{markdown.Frontmatter{Type: "Project"}, "project"},       // explicit, lower-cased
		{markdown.Frontmatter{Type: "page", Date: jan1}, "page"}, // type overrides the date
		{markdown.Frontmatter{Type: "post"}, "post"},             // type overrides dateless
	}
	for _, c := range cases {
		if got := resolvePageType(c.fm); got != c.want {
			t.Errorf("resolvePageType(%+v) = %q, want %q", c.fm, got, c.want)
		}
	}
}

func TestStandingType(t *testing.T) {
	if !standingType("page") {
		t.Error("page should be standing (nav)")
	}
	for _, listed := range []string{"post", "project", ""} {
		if standingType(listed) {
			t.Errorf("%q should be listed, not standing", listed)
		}
	}
}

// fakeEngine reports which templates exist, for templateFor.
type fakeEngine struct{ have map[string]bool }

func (f fakeEngine) Render(string, map[string]any) (string, error) { return "", nil }
func (f fakeEngine) HasTemplate(name string) bool                  { return f.have[name] }
func (f fakeEngine) Asset(string) ([]byte, error)                  { return nil, nil }
func (f fakeEngine) Assets() ([]string, error)                     { return nil, nil }

func TestTemplateFor(t *testing.T) {
	eng := fakeEngine{have: map[string]bool{"project.html": true}}
	var _ render.Engine = eng // fakeEngine satisfies the interface
	if got := templateFor(eng, "project"); got != "project.html" {
		t.Errorf("custom type with a template = %q, want project.html", got)
	}
	if got := templateFor(eng, "post"); got != "page.html" {
		t.Errorf("type without a template = %q, want page.html fallback", got)
	}
}

func TestReadingTime(t *testing.T) {
	words := func(n int) string { return strings.Repeat("word ", n) }
	// 370 words at 185 wpm = 120s = 2 min.
	if got := readingTime(words(370)); got != 2 {
		t.Errorf("370 words = %d, want 2", got)
	}
	// Visuals add 30s each: ~6s of prose + 4×30s = ~127s ≈ 2 min.
	if got := readingTime(words(20) + `<img><img><img><img>`); got != 2 {
		t.Errorf("20 words + 4 images = %d, want 2", got)
	}
	// A mermaid diagram counts as a visual too.
	if got := readingTime(`<pre class="mermaid">flowchart</pre>`); got != 1 {
		t.Errorf("one diagram = %d, want 1", got)
	}
	if got := readingTime(""); got != 1 {
		t.Errorf("empty = %d, want 1 (floor)", got)
	}
}

func TestIsEmptyContent(t *testing.T) {
	cases := []struct {
		html string
		want bool
	}{
		{"", true},
		{"   \n\t ", true},
		{"<p></p>", true}, // markup but no text
		{`<div class="callout"></div>`, true},
		{"<p>real content</p>", false},
		{"plain text", false},
	}
	for _, c := range cases {
		if got := isEmptyContent(c.html); got != c.want {
			t.Errorf("isEmptyContent(%q) = %v, want %v", c.html, got, c.want)
		}
	}
}

func TestTagLinks(t *testing.T) {
	got := tagLinks([]string{"Showcase", "tag 2", ""}, "/blog/")
	if len(got) != 2 { // the empty tag is dropped
		t.Fatalf("got %d links, want 2: %v", len(got), got)
	}
	if got[0]["name"] != "Showcase" || got[0]["url"] != "/blog/tags/showcase/" {
		t.Errorf("first tag = %v", got[0])
	}
	if got[1]["url"] != "/blog/tags/tag-2/" { // slugified
		t.Errorf("second tag url = %v", got[1]["url"])
	}
}

func TestResolveBasePath(t *testing.T) {
	tests := []struct {
		explicit string
		baseURL  string
		want     string
	}{
		{"", "https://example.com", "/"},
		{"", "https://example.com/", "/"},
		{"", "https://user.github.io/repo", "/repo/"},
		{"", "https://x.dev/a/b", "/a/b/"},
		{"", "", "/"},
		{"/main/preview/", "https://ignored.example", "/main/preview/"},
		{"repo", "", "/repo/"},
	}
	for _, tt := range tests {
		if got := resolveBasePath(tt.explicit, tt.baseURL); got != tt.want {
			t.Errorf("resolveBasePath(%q, %q) = %q, want %q", tt.explicit, tt.baseURL, got, tt.want)
		}
	}
}
