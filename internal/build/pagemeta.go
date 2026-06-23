package build

import (
	"math"
	"net/url"
	"regexp"
	"strings"

	"github.com/jmylchreest/colophon/internal/core"
)

// hHeadingRE matches an h2/h3 with an auto-generated id, for a table of contents.
var hHeadingRE = regexp.MustCompile(`(?s)<h([23])[^>]*\bid="([^"]+)"[^>]*>(.*?)</h[23]>`)

// readingTime estimates whole minutes to read rendered HTML: prose at wordsPerMinute, plus
// secondsPerVisual for each image or diagram in the body. Minimum 1. Tuning lives in
// pagemeta_constants.go.
func readingTime(html string) int {
	words := len(strings.Fields(tagRE.ReplaceAllString(html, " ")))
	visuals := strings.Count(html, "<img") + strings.Count(html, `class="mermaid"`)
	seconds := float64(words)/wordsPerMinute*60 + float64(visuals)*secondsPerVisual
	if m := int(math.Round(seconds / 60)); m > 1 {
		return m
	}
	return 1
}

// isEmptyContent reports whether rendered HTML carries no visible text — a uniform
// build-level structure check across all sources. A note may be selected for publishing
// (by a tag or the publish flag) yet still be a stub; the build warns but publishes it
// anyway (selection decides inclusion; format is only flagged).
func isEmptyContent(html string) bool {
	return strings.TrimSpace(tagRE.ReplaceAllString(html, "")) == ""
}

// tableOfContents extracts the h2/h3 headings (and their ids) from rendered HTML, for a
// theme's on-page contents sidebar. Each entry is {level, id, text}.
func tableOfContents(html string) []map[string]any {
	var toc []map[string]any
	for _, m := range hHeadingRE.FindAllStringSubmatch(html, -1) {
		text := strings.TrimSpace(tagRE.ReplaceAllString(m[3], ""))
		if text != "" {
			toc = append(toc, map[string]any{"level": m[1], "id": m[2], "text": text})
		}
	}
	return toc
}

// pageCategory is the page's primary category for themes that show one: the first category,
// else the first tag, else "".
func pageCategory(p page) string {
	if len(p.Categories) > 0 {
		return p.Categories[0]
	}
	if len(p.Tags) > 0 {
		return p.Tags[0]
	}
	return ""
}

// authorVars builds the byline/h-card template fields from a persona (nil → empty).
func authorVars(a core.Author) map[string]any {
	firstURL := ""
	if len(a.URLs) > 0 {
		firstURL = a.URLs[0]
	}
	return map[string]any{
		"author_name":         a.Name,
		"author_initials":     initials(a.Name),
		"author_bio":          a.Bio,
		"author_url":          firstURL,
		"author_avatar":       a.Avatar,
		"author_avatar_style": imageStyle(a.AvatarFit, a.AvatarPosition),
		"author_links":        authorLinks(a),
	}
}

// authorLinks turns an author's urls + email into renderable {url, label} pairs for the byline /
// author card: each url is labelled by its platform (GitHub, Bluesky, …) or bare host, and the
// email becomes a mailto. Order follows urls, with email last.
func authorLinks(a core.Author) []map[string]any {
	out := make([]map[string]any, 0, len(a.URLs)+1)
	for _, u := range a.URLs {
		if u = strings.TrimSpace(u); u == "" {
			continue
		}
		m := map[string]any{"url": u, "label": linkLabel(u)}
		// When the URL is a recognised silo, show its brand glyph (from the silos font) and use
		// the canonical network name — reusing the same detection as webmentions/syndication, so
		// author profiles, responses and "Also posted on" all read consistently.
		if id := siloForHost(hostOf(u)); id != "" && id != "website" {
			if g, ok := siloGlyph[id]; ok {
				m["silo"] = string(g)
				m["label"] = siloLabels[id]
			}
		}
		out = append(out, m)
	}
	if e := strings.TrimSpace(a.Email); e != "" {
		out = append(out, map[string]any{"url": "mailto:" + e, "label": "Email"})
	}
	return out
}

// authorsUseSilos reports whether any configured author has a profile link to a recognised silo,
// i.e. whether author links would render a glyph from the silos font. Used to decide whether to
// ship silos.woff2 even on sites without webmentions or syndication.
func authorsUseSilos(authors []core.Author) bool {
	for _, a := range authors {
		for _, l := range authorLinks(a) {
			if _, ok := l["silo"]; ok {
				return true
			}
		}
	}
	return false
}

// linkLabel names a URL by its platform, falling back to the bare host (or the raw string if it
// won't parse). Kept deliberately small — unknown hosts read fine as a domain.
func linkLabel(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	host := strings.ToLower(strings.TrimPrefix(u.Host, "www."))
	switch host {
	case "github.com":
		return "GitHub"
	case "gitlab.com":
		return "GitLab"
	case "x.com", "twitter.com":
		return "X"
	case "bsky.app":
		return "Bluesky"
	case "linkedin.com":
		return "LinkedIn"
	case "youtube.com", "youtu.be":
		return "YouTube"
	case "instagram.com":
		return "Instagram"
	case "mastodon.social", "hachyderm.io", "fosstodon.org":
		return "Mastodon"
	default:
		return host
	}
}

// initials returns up to two upper-case initials from a name.
func initials(name string) string {
	var b strings.Builder
	for _, f := range strings.Fields(name) {
		r := []rune(f)
		if len(r) == 0 {
			continue
		}
		b.WriteRune(r[0])
		if b.Len() >= 2 {
			break
		}
	}
	return strings.ToUpper(b.String())
}
