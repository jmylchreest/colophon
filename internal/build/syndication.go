package build

import (
	"fmt"
	"html"
	"net/url"
	"strings"
)

// normalizeSyndication trims, drops blanks, and de-duplicates the frontmatter
// syndication URL list (preserving order). These are absolute URLs where the post
// also lives (manual POSSE today; the syndication ledger appends here in Tier 3).
func normalizeSyndication(urls []string) []string {
	if len(urls) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(urls))
	out := make([]string, 0, len(urls))
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if _, dup := seen[u]; dup {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// syndicationHost returns a short, human label for a syndication URL — the host with
// a leading "www." stripped (e.g. "https://hachyderm.io/@me/123" → "hachyderm.io").
// Falls back to the raw string when it doesn't parse as a URL with a host.
func syndicationHost(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return strings.TrimPrefix(u.Host, "www.")
}

// syndicationHTML renders the engine "Also posted on…" block as microformats2
// u-syndication links — a no-JS drop-in (empty string when there are none), mirroring
// attachmentsHTML. Themes can use this or build their own from the `syndication` list.
func syndicationHTML(urls []string) string {
	if len(urls) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<nav class="post-syndication" aria-label="Also posted on">`)
	for _, u := range urls {
		fmt.Fprintf(&b, `<a class="u-syndication" rel="syndication" href="%s">%s</a>`,
			html.EscapeString(u), html.EscapeString(syndicationHost(u)))
	}
	b.WriteString(`</nav>`)
	return b.String()
}
