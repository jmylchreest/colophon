package build

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/webmention"
)

// mentionsJS is the shared browser renderer for webmention responses, emitted once at the site
// root (like player.js / search-ui.js) so any theme gets responses with just the placeholder
// markup. It reads data-mentions* attributes, fetches the source (our _mentions/<key>.json in
// asset mode, or the receiver's read API in live mode), and renders a facepile + replies.
//
//go:embed assets/mentions.js
var mentionsJS []byte

// mentionsCSS styles the responses block + syndication pills. Engine-provided (like glossary.css)
// so every theme gets a working, theme-token-aware look without duplicating CSS; emitted once at
// the site root alongside mentions.js. A theme may override any rule in its own stylesheet.
//
//go:embed assets/mentions.css
var mentionsCSS []byte

// silosWoff2 is the curated silo icon font (Bluesky/Mastodon/GitHub/… + a generic website globe),
// merged from Font Awesome packs by contrib/scripts/silo-font/build.py. Emitted once at the site
// root and declared @font-face by the themes; the glyph codepoints below mirror its silos.json.
//
//go:embed assets/silos.woff2
var silosWoff2 []byte

// mentionsBase is the output dir (and URL path) the per-post mention JSON lives under, mirroring
// _search/. Routed to the asset store alongside _search/ when routing is configured.
const mentionsBase = "_mentions"

// maxFaces caps the reactions facepile (likes/reposts) so a viral post renders a handful of
// avatars + a "+N" count rather than thousands. Shared with mentions.js (the live path).
const maxFaces = 16

// webmentionMode resolves the per-site display mode: "live", "asset", or "disabled" (default).
func webmentionMode(site core.Site) string {
	iw := site.Federation.IndieWeb
	if iw == nil || iw.Webmention == nil || iw.Webmention.Display == nil {
		return "disabled"
	}
	switch strings.ToLower(strings.TrimSpace(iw.Webmention.Display.Mode)) {
	case "live":
		return "live"
	case "asset":
		return "asset"
	default:
		return "disabled"
	}
}

// mentionsBaseURL is where the browser fetches _mentions/ from: the routed asset-store URL when
// _mentions/ is routed (mirroring searchBaseURL), else the local base_path. fetch()/CORS caveats
// match the search index — a routed cross-origin base needs the store to allow GET.
func mentionsBaseURL(router *core.Router, basePath string) string {
	if routed := router.AssetURL(mentionsBase + "/probe.json"); routed != "" {
		return strings.TrimSuffix(routed, "probe.json")
	}
	return basePath + mentionsBase + "/"
}

// mentionsAssetKey maps a post key to its asset basename ("" → "index"), matching the cache.
func mentionsAssetKey(key string) string {
	if key == "" {
		return "index"
	}
	return key
}

func mentionsAssetName(key string) string {
	return mentionsBase + "/" + mentionsAssetKey(key) + ".json"
}

// mentionsAttrs builds the data-* attributes for the theme placeholder, so the engine owns the
// fetch wiring and themes stay mode-agnostic. asset mode emits only data-mentions (our JSON URL);
// live mode adds data-mentions-live + the post URL as the target, plus the shipped glob blocklist
// (so client-side moderation works without a server). All values are attribute-escaped.
func mentionsAttrs(src, liveTarget, blockJSON string) string {
	attrs := fmt.Sprintf(`data-mentions="%s"`, html.EscapeString(src))
	if liveTarget != "" {
		attrs += fmt.Sprintf(` data-mentions-live data-mentions-target="%s"`, html.EscapeString(liveTarget))
	}
	if blockJSON != "" {
		attrs += fmt.Sprintf(` data-mentions-block="%s"`, html.EscapeString(blockJSON))
	}
	return attrs
}

// mentionVars maps mentions to the structured `mentions` template list (build-your-own).
func mentionVars(ms []webmention.Mention) []map[string]any {
	out := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		out = append(out, map[string]any{
			"type":      m.Type,
			"url":       m.URL,
			"content":   m.Content,
			"published": m.Published,
			"author": map[string]any{
				"name": m.Author.Name, "url": m.Author.URL, "photo": m.Author.Photo,
			},
		})
	}
	return out
}

// mentionsHTML renders the engine "responses" drop-in (no JS) for asset-mode bake: a compact
// reactions facepile (likes/reposts) plus a newest-first replies timeline, as mf2 h-cite. Empty
// string when there are none. The theme owns the <section class="responses"> wrapper.
func mentionsHTML(m webmention.Mentions) string {
	if len(m.Mentions) == 0 {
		return ""
	}
	ms := mentionsNewestFirst(m.Mentions)
	var faces, replies strings.Builder
	totalFaces, shownFaces := 0, 0
	for _, x := range ms {
		switch x.Type {
		case "like", "repost":
			totalFaces++
			if shownFaces < maxFaces { // cap the facepile so a viral post can't render thousands of avatars
				faces.WriteString(mentionFace(x))
				shownFaces++
			}
		default: // reply | mention
			replies.WriteString(mentionReply(x))
		}
	}
	var b strings.Builder
	b.WriteString(`<div class="responses-title">Responses</div>`)
	if shownFaces > 0 {
		facesHTML := faces.String()
		if totalFaces > maxFaces {
			facesHTML += fmt.Sprintf(`<li class="response-more">+%d</li>`, totalFaces-maxFaces)
		}
		b.WriteString(`<ul class="response-faces" aria-label="Reactions">` + facesHTML + `</ul>`)
	}
	if replies.Len() > 0 {
		b.WriteString(`<ol class="response-list">` + replies.String() + `</ol>`)
	}
	return b.String()
}

func mentionFace(x webmention.Mention) string {
	name := html.EscapeString(x.Author.Name)
	inner := name
	if name == "" {
		inner = "·"
	}
	if x.Author.Photo != "" {
		inner = fmt.Sprintf(`<img class="u-photo" src="%s" alt="%s" loading="lazy">`,
			html.EscapeString(x.Author.Photo), name)
	}
	verb := "reacted"
	switch x.Type {
	case "like":
		verb = "liked"
	case "repost":
		verb = "reposted"
	}
	link := x.Author.URL
	if link == "" {
		link = x.URL
	}
	title := strings.TrimSpace(x.Author.Name+" "+verb) + " this"
	return fmt.Sprintf(
		`<li class="response %s h-cite"><a class="p-author h-card u-url" href="%s" title="%s">%s</a></li>`,
		html.EscapeString(x.Type), html.EscapeString(link), html.EscapeString(title), inner)
}

// mentionReply renders one reply as a compact row mirroring the post-listing style: a left column
// with the source silo mark + date, and a right column with the author and a one-line content
// preview (the theme truncates it). Whole-content lives in the title for hover.
func mentionReply(x webmention.Mention) string {
	name := x.Author.Name
	if name == "" {
		name = "Someone"
	}
	var b strings.Builder
	b.WriteString(`<li class="response reply h-cite">`)

	// Left column: source silo + relative date, linking to the original. data-pop carries the
	// network + full timestamp for the themed hover tooltip.
	if x.URL != "" {
		glyph, label := siloMark(mentionHost(x))
		var parts []string
		if label != "" {
			parts = append(parts, label)
		}
		if fd := fullDate(x.Published); fd != "" {
			parts = append(parts, fd)
		}
		b.WriteString(`<a class="response-perma u-url" href="` + html.EscapeString(x.URL) + `"`)
		if pop := strings.Join(parts, " · "); pop != "" {
			b.WriteString(` data-pop="` + html.EscapeString(pop) + `"`)
		}
		b.WriteString(`>`)
		if glyph != 0 {
			b.WriteString(`<span class="silo" aria-hidden="true">` + string(glyph) + `</span>`)
		}
		if r := relativeDate(x.Published); r != "" {
			b.WriteString(`<time class="dt-published" datetime="` + html.EscapeString(x.Published) + `">` + html.EscapeString(r) + `</time>`)
		} else if glyph == 0 {
			b.WriteString(`<span class="response-go" aria-hidden="true">↗</span>`)
		}
		b.WriteString(`</a>`)
	}

	// Right column: author + one-line content preview (full content in data-pop for the tooltip).
	b.WriteString(`<div class="response-body"`)
	if x.Content != "" {
		b.WriteString(` data-pop="` + html.EscapeString(x.Content) + `"`)
	}
	b.WriteString(`>`)
	b.WriteString(`<a class="p-author h-card u-url" href="` + html.EscapeString(authorLink(x)) + `">`)
	if x.Author.Photo != "" {
		b.WriteString(`<img class="u-photo" src="` + html.EscapeString(x.Author.Photo) + `" alt="" loading="lazy">`)
	}
	b.WriteString(`<span class="p-name">` + html.EscapeString(name) + `</span></a>`)
	if x.Content != "" {
		b.WriteString(`<span class="p-content">` + html.EscapeString(x.Content) + `</span>`)
	}
	b.WriteString(`</div></li>`)
	return b.String()
}

func authorLink(x webmention.Mention) string {
	if x.Author.URL != "" {
		return x.Author.URL
	}
	return x.URL
}

// mentionsNewestFirst returns a copy sorted by published descending (unparseable dates last).
func mentionsNewestFirst(ms []webmention.Mention) []webmention.Mention {
	out := append([]webmention.Mention(nil), ms...)
	sort.SliceStable(out, func(i, j int) bool {
		return mentionTime(out[i].Published).After(mentionTime(out[j].Published))
	})
	return out
}

func mentionTime(s string) time.Time {
	for _, f := range []string{time.RFC3339, "2006-01-02T15:04:05Z07:00", "2006-01-02"} {
		if t, err := time.Parse(f, strings.TrimSpace(s)); err == nil {
			return t
		}
	}
	return time.Time{}
}

// relativeDate formats a mention timestamp as a compact relative age ("3h", "2d", "5mo", "2y",
// or "now"); "" when unparseable/empty. Built at render time, so it's as-of-build for baked pages
// and live for the JS path (which recomputes it client-side).
func relativeDate(s string) string {
	t := mentionTime(s)
	if t.IsZero() {
		return ""
	}
	d := buildNow().Sub(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}
}

// fullDate is the readable absolute timestamp for the tooltip (with time when the source has one).
func fullDate(s string) string {
	t := mentionTime(s)
	if t.IsZero() {
		return ""
	}
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
		return t.Format("2 Jan 2006")
	}
	return t.Format("2 Jan 2006, 15:04")
}

// buildNow is the reference instant for relative dates; a var so it stays stable within a build.
var buildNow = time.Now

// mentionHost is the source host (the silo the mention came from) — the source URL's host, else
// the author URL's.
func mentionHost(x webmention.Mention) string {
	if h := hostOf(x.URL); h != "" {
		return h
	}
	return hostOf(x.Author.URL)
}

func hostOf(raw string) string {
	if u, err := url.Parse(raw); err == nil {
		return strings.TrimPrefix(u.Host, "www.")
	}
	return ""
}

// knownMastodon is a small set of popular instances, since Mastodon can't be detected by host
// shape; other fediverse/unknown hosts fall through to the generic website glyph.
var knownMastodon = map[string]bool{
	"hachyderm.io": true, "fosstodon.org": true, "mas.to": true, "mstdn.social": true,
	"infosec.exchange": true, "social.coop": true, "techhub.social": true, "indieweb.social": true,
}

// siloGlyph maps a silo id to its codepoint in silos.woff2. KEEP IN SYNC with
// contrib/scripts/silo-font/silos.json — regenerate both via that script when a silo changes.
// (square variants also ship: bluesky-square F301, github-square F304, x-square F306.)
var siloGlyph = map[string]rune{
	"bluesky":    '\uf300',
	"mastodon":   '\uf301',
	"github":     '\uf302',
	"x":          '\uf303',
	"reddit":     '\uf304',
	"hackernews": '\uf305',
	"threads":    '\uf306',
	"flickr":     '\uf307',
	"linkedin":   '\uf308',
	"tumblr":     '\uf309',
	"gitlab":     '\uf30a',
	"website":    '\uf30b',
}

var siloLabels = map[string]string{
	"bluesky": "Bluesky", "mastodon": "Mastodon", "github": "GitHub", "x": "X",
	"reddit": "Reddit", "hackernews": "Hacker News", "threads": "Threads", "flickr": "Flickr",
	"linkedin": "LinkedIn", "tumblr": "Tumblr", "gitlab": "GitLab", "website": "Website",
}

// siloForHost maps a source host to a silo id. Single-domain silos are matched exactly; Mastodon
// is heuristic (multi-instance); any other http(s) host falls back to the generic "website".
func siloForHost(host string) string {
	h := strings.ToLower(host)
	switch {
	case h == "":
		return ""
	case strings.Contains(h, "bsky."):
		return "bluesky"
	case h == "github.com" || strings.HasSuffix(h, ".github.com"):
		return "github"
	case h == "gitlab.com":
		return "gitlab"
	case h == "reddit.com" || strings.HasSuffix(h, ".reddit.com"):
		return "reddit"
	case h == "news.ycombinator.com":
		return "hackernews"
	case h == "threads.net" || strings.HasSuffix(h, ".threads.net"):
		return "threads"
	case h == "flickr.com" || strings.HasSuffix(h, ".flickr.com"):
		return "flickr"
	case h == "linkedin.com" || strings.HasSuffix(h, ".linkedin.com"):
		return "linkedin"
	case h == "tumblr.com" || strings.HasSuffix(h, ".tumblr.com"):
		return "tumblr"
	case h == "x.com" || h == "twitter.com" || strings.HasSuffix(h, ".x.com") || strings.HasSuffix(h, ".twitter.com"):
		return "x"
	case strings.Contains(h, "mastodon") || strings.Contains(h, "mstdn") || knownMastodon[h]:
		return "mastodon"
	default:
		return "website"
	}
}

// siloMark returns the silo font glyph + human label for a source host (0/"" only for an empty host).
func siloMark(host string) (rune, string) {
	id := siloForHost(host)
	if id == "" {
		return 0, ""
	}
	g, ok := siloGlyph[id]
	if !ok {
		return 0, ""
	}
	return g, siloLabels[id]
}

func mentionsAssetJSON(m webmention.Mentions) ([]byte, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
