package build

import (
	"html"
	"net/url"
	"strings"

	"github.com/jmylchreest/colophon/internal/syndicate"
)

// pageSyndication merges a post's manual syndication: frontmatter with the silo URLs the
// syndication ledger recorded for it (deduped, frontmatter first), for the u-syndication links.
func pageSyndication(p page, ledger *syndicate.Ledger) []string {
	urls := p.Syndication
	if ledger != nil {
		if extra := ledger.URLs(strings.Trim(p.URL, "/")); len(extra) > 0 {
			urls = append(append([]string{}, p.Syndication...), extra...)
		}
	}
	return normalizeSyndication(urls)
}

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

// syndicationHTML renders the engine "Also posted on…" block as microformats2 u-syndication
// links — a labelled row of chips, each with the silo's icon + network name (e.g. "Bluesky")
// when recognised, else a globe + the host. No-JS drop-in; empty string when there are none.
func syndicationHTML(urls []string) string {
	if len(urls) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<nav class="post-syndication" aria-label="Also posted on">`)
	b.WriteString(`<span class="syn-label">Also posted on</span>`)
	for _, u := range urls {
		host := syndicationHost(u)
		id := siloForHost(host)
		text := siloLabels[id] // "Bluesky", "GitHub", … ; "Website" for the generic
		if id == "website" || id == "" {
			text = host // for unknown hosts the domain is more useful than "Website"
		}
		b.WriteString(`<a class="u-syndication syn-link" rel="syndication" href="` + html.EscapeString(u) + `" title="` + html.EscapeString(text) + `">`)
		if g := siloGlyph[id]; g != 0 {
			b.WriteString(`<span class="silo" aria-hidden="true">` + string(g) + `</span>`)
		}
		b.WriteString(`<span class="syn-name">` + html.EscapeString(text) + `</span></a>`)
	}
	b.WriteString(`</nav>`)
	return b.String()
}
