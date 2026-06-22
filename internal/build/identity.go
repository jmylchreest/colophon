package build

import (
	"html"
	"strings"
)

// relMeLinks returns <link rel="me"> tags for an author's identity URLs (all of them), for
// the <head> of pages that author owns: their posts and their author feed page. Listing
// pages with no single author (home, per-tag) carry none, so an author's rel="me" never
// leaks onto another author's page — keeping the rel-me identity graph per-author.
//
// rel="me" is what IndieAuth/IndieLogin (e.g. webmention.io sign-in) reads to verify an
// identity, so the page that carries it (the author feed page, or a post) is the URL to sign
// in with — and the provider it points to must link back to that same URL. Empty when the
// author has no urls:.
func relMeLinks(urls []string) string {
	var b strings.Builder
	for _, u := range urls {
		if u = strings.TrimSpace(u); u != "" {
			b.WriteString(`<link rel="me" href="` + html.EscapeString(u) + `">`)
		}
	}
	return b.String()
}
