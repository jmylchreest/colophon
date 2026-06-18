package build

import (
	"math"
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
	url := ""
	if len(a.URLs) > 0 {
		url = a.URLs[0]
	}
	return map[string]any{
		"author_name":         a.Name,
		"author_initials":     initials(a.Name),
		"author_bio":          a.Bio,
		"author_url":          url,
		"author_avatar":       a.Avatar,
		"author_avatar_style": imageStyle(a.AvatarFit, a.AvatarPosition),
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
