package build

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jmylchreest/colophon/internal/clog"
)

func TestNormalizeAliases(t *testing.T) {
	got := normalizeAliases([]string{"Old Name", "2020/Legacy Post", "   ", "a//b"})
	want := []string{"old-name", "2020/legacy-post", "a/b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("normalizeAliases = %v, want %v", got, want)
	}
}

func TestEmitRedirects(t *testing.T) {
	pages := []page{
		{Slug: "posts/renamed", URL: "posts/renamed/", Aliases: []string{"old-name", "2020/legacy-post"}},
		{Slug: "posts/other", URL: "posts/other/", Aliases: []string{"old-name"}},      // duplicate alias → first wins
		{Slug: "posts/clash", URL: "posts/clash/", Aliases: []string{"posts/renamed"}}, // collides with a real page
	}
	files := map[string][]byte{}
	write := func(rel string, b []byte) error { files[rel] = b; return nil }

	if err := emitRedirects(write, pages, "/", "https://x.test", clog.Discard()); err != nil {
		t.Fatal(err)
	}

	for _, k := range []string{"old-name/index.html", "2020/legacy-post/index.html", ".nojekyll", "_redirects"} {
		if _, ok := files[k]; !ok {
			t.Errorf("expected %q to be written", k)
		}
	}
	// An alias must never clobber a real page.
	if _, ok := files["posts/renamed/index.html"]; ok {
		t.Error("alias collided with a real page and overwrote it")
	}

	red := string(files["_redirects"])
	if !strings.Contains(red, "/old-name/ /posts/renamed/ 301") {
		t.Errorf("_redirects missing rule: %q", red)
	}
	if strings.Count(red, "/old-name/ ") != 1 {
		t.Errorf("duplicate alias not deduped: %q", red)
	}

	stub := string(files["old-name/index.html"])
	for _, want := range []string{
		`rel="canonical" href="https://x.test/posts/renamed/"`,
		`content="0; url=/posts/renamed/"`,
		`name="robots" content="noindex"`,
		`location.replace("/posts/renamed/")`,
	} {
		if !strings.Contains(stub, want) {
			t.Errorf("stub missing %q in:\n%s", want, stub)
		}
	}
}
