package build

import "testing"

func TestResolveWikilinks(t *testing.T) {
	r := linkResolver{}
	r.add("posts/hello-world.md", "posts/hello-world", "/")
	r.add("about.md", "about", "/main/preview/")

	tests := []struct {
		in   string
		want string
	}{
		{"see [[hello-world]]", "see [hello-world](/posts/hello-world/)"},
		{"see [[posts/hello-world]]", "see [posts/hello-world](/posts/hello-world/)"},
		{"see [[hello-world|the intro]]", "see [the intro](/posts/hello-world/)"},
		{"jump [[hello-world#A Heading]]", "jump [hello-world](/posts/hello-world/#a-heading)"},
		{"under prefix [[about]]", "under prefix [about](/main/preview/about/)"},
		{"missing [[no-such-note]]", "missing no-such-note"},
		{"missing alias [[no-such|label]]", "missing alias label"},
		{"embed left alone ![[image.png]]", "embed left alone ![[image.png]]"},
		{"plain text no links", "plain text no links"},
	}
	for _, tt := range tests {
		if got := resolveWikilinks(tt.in, r); got != tt.want {
			t.Errorf("resolveWikilinks(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
