package build

import (
	"strings"
	"testing"
)

func TestNormalizeSyndication(t *testing.T) {
	got := normalizeSyndication([]string{
		" https://a.example/1 ", "https://a.example/1", "", "https://b.example/2", "  ",
	})
	want := []string{"https://a.example/1", "https://b.example/2"}
	if len(got) != len(want) {
		t.Fatalf("normalizeSyndication = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if normalizeSyndication(nil) != nil {
		t.Error("nil input should give nil")
	}
	if normalizeSyndication([]string{"  ", ""}) != nil {
		t.Error("all-blank input should give nil")
	}
}

func TestSyndicationHost(t *testing.T) {
	cases := map[string]string{
		"https://hachyderm.io/@me/123": "hachyderm.io",
		"https://www.example.com/p":    "example.com",
		"not a url":                    "not a url",
	}
	for in, want := range cases {
		if got := syndicationHost(in); got != want {
			t.Errorf("syndicationHost(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSyndicationHTML(t *testing.T) {
	if syndicationHTML(nil) != "" {
		t.Error("no URLs should render empty")
	}
	got := syndicationHTML([]string{"https://hachyderm.io/@me/1", "https://bsky.app/x?a=1&b=2"})
	for _, want := range []string{
		`class="u-syndication"`, `rel="syndication"`,
		`href="https://hachyderm.io/@me/1"`, `>hachyderm.io<`,
		`a=1&amp;b=2`, // query ampersand escaped
	} {
		if !strings.Contains(got, want) {
			t.Errorf("syndicationHTML missing %q in:\n%s", want, got)
		}
	}
}
