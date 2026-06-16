package build

import (
	"strings"
	"testing"
)

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
