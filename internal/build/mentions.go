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

// mentionsBase is the output dir (and URL path) the per-post mention JSON lives under, mirroring
// _search/. Routed to the asset store alongside _search/ when routing is configured.
const mentionsBase = "_mentions"

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
	nFaces := 0
	for _, x := range ms {
		switch x.Type {
		case "like", "repost":
			nFaces++
			faces.WriteString(mentionFace(x))
		default: // reply | mention
			replies.WriteString(mentionReply(x))
		}
	}
	var b strings.Builder
	b.WriteString(`<div class="responses-title">Responses</div>`)
	if nFaces > 0 {
		b.WriteString(`<ul class="response-faces" aria-label="Reactions">` + faces.String() + `</ul>`)
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

func mentionReply(x webmention.Mention) string {
	name := x.Author.Name
	if name == "" {
		name = "Someone"
	}
	var b strings.Builder
	b.WriteString(`<li class="response reply h-cite">`)
	b.WriteString(`<a class="p-author h-card u-url" href="` + html.EscapeString(authorLink(x)) + `">`)
	if x.Author.Photo != "" {
		b.WriteString(`<img class="u-photo" src="` + html.EscapeString(x.Author.Photo) + `" alt="" loading="lazy">`)
	}
	b.WriteString(`<span class="p-name">` + html.EscapeString(name) + `</span></a>`)
	// Source + date, linking to the original. The silo mark shows only when we recognise it for
	// certain; otherwise it's omitted (no generic icon) and just the date/affordance remains.
	if x.URL != "" {
		icon, label := siloIcon(mentionHost(x))
		b.WriteString(`<a class="response-perma u-url" href="` + html.EscapeString(x.URL) + `"`)
		if label != "" {
			b.WriteString(` title="` + html.EscapeString(label) + `"`)
		}
		b.WriteString(`>`)
		if icon != "" {
			b.WriteString(`<span class="silo">` + icon + `</span>`)
		}
		d := shortDate(x.Published)
		switch {
		case d != "":
			b.WriteString(`<time class="dt-published" datetime="` + html.EscapeString(x.Published) + `">` + html.EscapeString(d) + `</time>`)
		case icon == "":
			b.WriteString(`<span class="response-go" aria-hidden="true">↗</span>`)
		}
		b.WriteString(`</a>`)
	}
	if x.Content != "" {
		b.WriteString(`<div class="p-content">` + html.EscapeString(x.Content) + `</div>`)
	}
	b.WriteString(`</li>`)
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

// shortDate formats a mention timestamp compactly (e.g. "2 Jan 2006"); "" when unparseable/empty.
func shortDate(s string) string {
	if t := mentionTime(s); !t.IsZero() {
		return t.Format("2 Jan 2006")
	}
	return ""
}

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
// shape; anything else unrecognised falls through to the generic icon + host text.
var knownMastodon = map[string]bool{
	"hachyderm.io": true, "fosstodon.org": true, "mas.to": true, "mstdn.social": true,
	"infosec.exchange": true, "social.coop": true, "techhub.social": true, "indieweb.social": true,
}

// siloIcon returns the inline-SVG network icon + label for a source host, but ONLY when the silo
// is recognised for certain. Unknown hosts return ("", "") so the renderer shows no icon (and no
// generic/host fallback) — keeping the date/link placement intact.
func siloIcon(host string) (svg, label string) {
	h := strings.ToLower(host)
	switch {
	case strings.Contains(h, "bsky."):
		return icoBluesky, "Bluesky"
	case h == "github.com" || strings.HasSuffix(h, ".github.com"):
		return icoGitHub, "GitHub"
	case h == "x.com" || h == "twitter.com" || strings.HasSuffix(h, ".x.com") || strings.HasSuffix(h, ".twitter.com"):
		return icoX, "X"
	case strings.Contains(h, "mastodon") || strings.Contains(h, "mstdn") || knownMastodon[h]:
		return icoMastodon, "Mastodon"
	default:
		return "", ""
	}
}

// Inline SVG silo marks (currentColor), kept small and brand-recognisable. Shared visual language
// with assets/mentions.js (the live path renders the same set).
const (
	icoBluesky  = `<svg viewBox="0 0 600 530" aria-hidden="true"><path fill="currentColor" d="M135 44c66 50 137 151 163 205 26-54 97-155 163-205 48-36 126-64 126 25 0 18-10 150-16 171-21 73-95 91-161 80 115 20 144 85 81 150-120 124-172-31-185-66-2-6-3-9-3-7 0-2-1 1-3 7-13 35-65 190-185 66-63-65-34-130 81-150-66 11-140-7-161-80-6-21-16-153-16-171 0-89 78-61 126-25Z"/></svg>`
	icoMastodon = `<svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M23.27 5.3c-.36-2.66-2.69-4.76-5.45-5.17C17.36.06 15.6 0 12 0h-.03C8.37 0 7.6.06 7.14.13 4.46.53 2.01 2.42 1.42 5.11.83 7.81.77 10.8.88 13.55c.16 3.93.2 4.62.97 6.79.69 1.93 2.62 3.41 4.7 3.99 2.28.64 4.73.75 7.06.31.32-.06.64-.13.95-.21l-.04-2.07s-1.6.37-3.4.31c-1.78-.06-3.66-.19-3.95-2.38a4.5 4.5 0 0 1-.04-.61s1.75.42 3.96.52c1.35.06 2.62-.08 3.91-.23 2.48-.3 4.64-1.82 4.91-3.21.43-2.19.4-5.35.4-5.35 0-3.1-2.05-4.01-2.05-4.01M19.62 14.5h-2.28v-5.6c0-1.17-.49-1.77-1.48-1.77-1.09 0-1.64.71-1.64 2.1v3.04H11.96V9.23c0-1.39-.55-2.1-1.64-2.1-.99 0-1.48.6-1.48 1.77v5.6H6.56V8.73c0-1.17.3-2.1.9-2.79.61-.69 1.42-1.04 2.42-1.04 1.16 0 2.04.45 2.62 1.34l.57.95.57-.95c.58-.89 1.46-1.34 2.62-1.34 1 0 1.81.35 2.42 1.04.6.69.9 1.62.9 2.79z"/></svg>`
	icoGitHub   = `<svg viewBox="0 0 16 16" aria-hidden="true"><path fill="currentColor" d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.6 7.6 0 0 1 2-.27c.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"/></svg>`
	icoX        = `<svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M18.244 2.25h3.308l-7.227 8.26 8.502 11.24H16.17l-5.214-6.817L4.99 21.75H1.68l7.73-8.835L1.254 2.25H8.08l4.713 6.231Zm-1.161 17.52h1.833L7.084 4.126H5.117Z"/></svg>`
)

func mentionsAssetJSON(m webmention.Mentions) ([]byte, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
