package build

import "testing"

func TestSlugFor(t *testing.T) {
	tests := []struct {
		rel, override, want string
	}{
		{"posts/hello-world.md", "", "posts/hello-world"},
		{"archive/template 1.md", "", "archive/template-1"},
		{"Archive/My Great Post.md", "", "archive/my-great-post"},
		{"notes/Idea (v2).md", "", "notes/idea-v2"},
		{"a/b/index.md", "", "a/b"},
		{"posts/whatever.md", "Custom Slug!", "posts/custom-slug"},
		{"Café Notes/Über.md", "", "caf-notes/ber"}, // non-ASCII dropped (ASCII-only slugs)
		{"posts/--weird__name--.md", "", "posts/weird-name"},
	}
	for _, tt := range tests {
		if got := slugFor(tt.rel, tt.override); got != tt.want {
			t.Errorf("slugFor(%q, %q) = %q, want %q", tt.rel, tt.override, got, tt.want)
		}
	}
}
