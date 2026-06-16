package build

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/feed"
)

// feedSpec maps a configured feed name to its output file and MIME type.
var feedSpec = map[string]struct {
	file string
	mime string
}{
	"rss":  {"rss.xml", "application/rss+xml"},
	"atom": {"atom.xml", "application/atom+xml"},
	"json": {"feed.json", "application/feed+json"},
}

// feedFormats is the feeds to emit for a site: its configured federation.feeds, or
// rss+atom+json by default (PLAN §10: always RSS + Atom + JSON Feed).
func feedFormats(site core.Site) []string {
	if len(site.Federation.Feeds) > 0 {
		return site.Federation.Feeds
	}
	return []string{"rss", "atom", "json"}
}

// feedLabels are the visible link labels per format.
var feedLabels = map[string]string{"rss": "RSS", "atom": "Atom", "json": "JSON"}

// FeedURLs returns absolute URLs for a site's feeds keyed by format, for callers (e.g. a
// publisher's site manifest) that need to link to them outside the build itself.
func FeedURLs(site core.Site, baseURL string) map[string]string {
	base := strings.TrimRight(baseURL, "/")
	out := map[string]string{}
	for _, f := range feedFormats(site) {
		if spec, ok := feedSpec[f]; ok {
			out[f] = base + "/" + spec.file
		}
	}
	return out
}

// feedLinks returns visible {label, href} entries for on-page subscribe links. Hrefs
// are base_path-relative so they resolve under serve's /<site>/<env>/ prefix too.
func feedLinks(formats []string, basePath string) []map[string]any {
	var links []map[string]any
	for _, f := range formats {
		spec, ok := feedSpec[f]
		if !ok {
			continue
		}
		links = append(links, map[string]any{"label": feedLabels[f], "href": basePath + spec.file})
	}
	return links
}

// feedDiscoveryLinks returns the <link rel="alternate"> autodiscovery tags for the head.
func feedDiscoveryLinks(site core.Site, formats []string) string {
	base := strings.TrimRight(site.BaseURL, "/")
	var b strings.Builder
	for _, f := range formats {
		spec, ok := feedSpec[f]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, `<link rel="alternate" type="%s" title="%s" href="%s/%s">`,
			spec.mime, html.EscapeString(site.Title), base, spec.file)
	}
	return b.String()
}

// writeFeeds generates the configured feeds plus a sitemap and writes them. The feed
// includes dated pages (posts); the sitemap includes every page plus the home URL.
func writeFeeds(write func(string, []byte) error, site core.Site, cfg *config.Config, formats []string, pages []page) error {
	base := strings.TrimRight(site.BaseURL, "/")

	var items []feed.Item
	sitemap := []feed.SitemapEntry{{URL: base + "/"}}
	for _, p := range pages {
		abs := base + "/" + p.URL
		sitemap = append(sitemap, feed.SitemapEntry{URL: abs, LastMod: p.Published})
		// Feed items are chronological posts (standing pages like About are nav chrome, not
		// feed entries) that carry a date to order by.
		if !p.Static && !p.Published.IsZero() {
			items = append(items, feed.Item{
				Title:       p.Title,
				URL:         abs,
				Description: p.Description,
				Content:     p.HTML,
				Published:   p.Published,
			})
		}
	}

	fs := feed.Site{Title: site.Title, BaseURL: base, Author: feedAuthor(cfg)}
	renderers := map[string]func(feed.Site, []feed.Item) ([]byte, error){
		"rss": feed.RSS, "atom": feed.Atom, "json": feed.JSON,
	}
	for _, f := range formats {
		spec, ok := feedSpec[f]
		if !ok {
			continue
		}
		data, err := renderers[f](fs, items)
		if err != nil {
			return fmt.Errorf("render %s feed: %w", f, err)
		}
		if err := write(spec.file, data); err != nil {
			return err
		}
	}

	sm, err := feed.Sitemap(sitemap)
	if err != nil {
		return err
	}
	if err := write("sitemap.xml", sm); err != nil {
		return err
	}

	// robots.txt makes the sitemap discoverable to crawlers (the standard mechanism).
	robots := fmt.Sprintf("User-agent: *\nAllow: /\n\nSitemap: %s/sitemap.xml\n", base)
	return write("robots.txt", []byte(robots))
}

// feedAuthor derives a byline from the first configured persona, if any.
func feedAuthor(cfg *config.Config) string {
	if len(cfg.Personas) == 0 {
		return ""
	}
	p := cfg.Personas[0]
	if p.Byline != "" {
		return p.Byline
	}
	return p.DisplayName
}

var tagRE = regexp.MustCompile(`<[^>]*>`)

// excerpt strips HTML tags from body and returns up to n runes of plain text.
func excerpt(body string, n int) string {
	text := strings.TrimSpace(html.UnescapeString(tagRE.ReplaceAllString(body, "")))
	text = strings.Join(strings.Fields(text), " ")
	r := []rune(text)
	if len(r) <= n {
		return text
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}
