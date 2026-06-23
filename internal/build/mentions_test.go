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
		`class="responses-title"`, `response-faces`, `response-replies`,
		`class="response like h-cite"`, `u-photo`,
		`class="p-content">Nice &amp; sharp<`, // content HTML-escaped
		`Bob &lt;b&gt;`,                       // author name escaped
		`dt-published">2026-06-22<`,
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
