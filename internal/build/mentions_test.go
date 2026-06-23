package build

import (
	"strings"
	"testing"

	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/webmention"
)

func wmSite(mode string) core.Site {
	return core.Site{Federation: core.Federation{IndieWeb: &core.IndieWeb{
		Webmention: &core.WebmentionConf{Display: &core.WebmentionDisplay{Mode: mode}},
	}}}
}

func TestWebmentionMode(t *testing.T) {
	cases := map[string]string{"asset": "asset", "LIVE": "live", "  asset ": "asset", "nonsense": "disabled", "": "disabled"}
	for in, want := range cases {
		if got := webmentionMode(wmSite(in)); got != want {
			t.Errorf("webmentionMode(%q) = %q, want %q", in, got, want)
		}
	}
	// No federation/indieweb/webmention/display → disabled.
	if got := webmentionMode(core.Site{}); got != "disabled" {
		t.Errorf("empty site = %q, want disabled", got)
	}
}

func TestMentionsAssetName(t *testing.T) {
	if got := mentionsAssetName("posts/x"); got != "_mentions/posts/x.json" {
		t.Errorf("asset name = %q", got)
	}
	if got := mentionsAssetName(""); got != "_mentions/index.json" {
		t.Errorf("home asset name = %q", got)
	}
}

func TestMentionsHTML(t *testing.T) {
	if mentionsHTML(webmention.Mentions{}) != "" {
		t.Error("no mentions → empty")
	}
	m := webmention.Mentions{Mentions: []webmention.Mention{
		{Type: "like", Author: webmention.MentionAuthor{Name: "Ada", URL: "https://ada.example", Photo: "https://ada.example/a.jpg"}, URL: "https://ada.example/l/1"},
		{Type: "reply", Author: webmention.MentionAuthor{Name: "Bob <b>", URL: "https://bob.example"}, URL: "https://bob.example/n/2", Content: "Nice & sharp", Published: "2026-06-22"},
	}}
	got := mentionsHTML(m)
	for _, want := range []string{
		`class="responses-title"`, `response-faces`, `response-list`, `response-body`,
		`class="response like h-cite"`, `u-photo`,
		`class="p-content"`, `>Nice &amp; sharp<`, // content span + HTML-escaped text
		`Bob &lt;b&gt;`,         // author name escaped
		`datetime="2026-06-22"`, // raw ISO in the datetime attr
		`>22 Jun 2026<`,         // human date rendered
	} {
		if !strings.Contains(got, want) {
			t.Errorf("mentionsHTML missing %q in:\n%s", want, got)
		}
	}
}

func TestMentionVars(t *testing.T) {
	vars := mentionVars([]webmention.Mention{{Type: "reply", Author: webmention.MentionAuthor{Name: "X"}, URL: "u", Content: "c"}})
	if len(vars) != 1 || vars[0]["type"] != "reply" || vars[0]["content"] != "c" {
		t.Fatalf("mentionVars = %v", vars)
	}
	if a, ok := vars[0]["author"].(map[string]any); !ok || a["name"] != "X" {
		t.Errorf("author var = %v", vars[0]["author"])
	}
}

func TestKeyForURL(t *testing.T) {
	cases := map[string]string{
		"https://blog.example.com/posts/x/": "posts/x",
		"/posts/x/":                         "posts/x",
		"https://blog.example.com/":         "",
		"":                                  "",
	}
	for in, want := range cases {
		if got := webmention.KeyForURL(in); got != want {
			t.Errorf("KeyForURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSiloForHost(t *testing.T) {
	cases := map[string]string{
		"bsky.app": "bluesky", "github.com": "github", "gist.github.com": "github",
		"reddit.com": "reddit", "news.ycombinator.com": "hackernews", "threads.net": "threads",
		"flickr.com": "flickr", "linkedin.com": "linkedin", "me.tumblr.com": "tumblr",
		"gitlab.com": "gitlab", "x.com": "x", "twitter.com": "x",
		"hachyderm.io": "mastodon", "mstdn.social": "mastodon",
		"random.example": "website", "": "",
	}
	for h, want := range cases {
		if got := siloForHost(h); got != want {
			t.Errorf("siloForHost(%q) = %q, want %q", h, got, want)
		}
	}
	if g, l := siloMark("bsky.app"); g != '\uf300' || l != "Bluesky" {
		t.Errorf("siloMark(bsky) = %U %q", g, l)
	}
	if g, l := siloMark("random.example"); g != '\uf30e' || l != "Website" {
		t.Errorf("siloMark(unknown) = %U %q", g, l)
	}
	if g, _ := siloMark(""); g != 0 {
		t.Error("empty host should yield no glyph")
	}
}
