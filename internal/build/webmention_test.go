package build

import (
	"strings"
	"testing"

	"github.com/jmylchreest/colophon/internal/core"
)

func TestWebmentionHead(t *testing.T) {
	const endpoint = "https://webmention.io/blog.example.com/webmention"

	withReceiver := core.Site{
		Federation: core.Federation{
			IndieWeb: &core.IndieWeb{
				Webmention: &core.WebmentionConf{Receiver: endpoint},
			},
		},
	}
	got := webmentionHead(withReceiver)
	want := `<link rel="webmention" href="` + endpoint + `">`
	if got != want {
		t.Errorf("webmentionHead with receiver = %q, want %q", got, want)
	}

	// No federation / indieweb / webmention config → no tag (the common case).
	for name, site := range map[string]core.Site{
		"empty":         {},
		"no-indieweb":   {Federation: core.Federation{}},
		"no-webmention": {Federation: core.Federation{IndieWeb: &core.IndieWeb{}}},
		"blank-receiver": {Federation: core.Federation{
			IndieWeb: &core.IndieWeb{Webmention: &core.WebmentionConf{Receiver: "  "}},
		}},
	} {
		if got := webmentionHead(site); got != "" {
			t.Errorf("webmentionHead(%s) = %q, want empty", name, got)
		}
	}
}

func TestWebmentionHeadEscapes(t *testing.T) {
	site := core.Site{Federation: core.Federation{IndieWeb: &core.IndieWeb{
		Webmention: &core.WebmentionConf{Receiver: "https://x.example/wm?a=1&b=2"},
	}}}
	got := webmentionHead(site)
	if strings.Contains(got, "a=1&b=2") {
		t.Errorf("webmentionHead did not escape the ampersand: %q", got)
	}
	if !strings.Contains(got, "a=1&amp;b=2") {
		t.Errorf("webmentionHead = %q, want escaped &amp;", got)
	}
}
