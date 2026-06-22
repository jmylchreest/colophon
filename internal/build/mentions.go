package build

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html"
	"strings"

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

// mentionsHTML renders the engine "responses" drop-in (no JS) for asset-mode bake: a facepile of
// likes/reposts plus reply cards, as mf2 h-cite. Empty string when there are none. Mirrors
// attachmentsHTML; themes can use it or build their own from `mentions`.
func mentionsHTML(m webmention.Mentions) string {
	if len(m.Mentions) == 0 {
		return ""
	}
	var faces, replies strings.Builder
	nFaces := 0
	for _, x := range m.Mentions {
		switch x.Type {
		case "like", "repost":
			nFaces++
			faces.WriteString(mentionFace(x))
		default: // reply | mention
			replies.WriteString(mentionReply(x))
		}
	}
	var b strings.Builder
	b.WriteString(`<section class="responses" aria-label="Responses">`)
	if nFaces > 0 {
		b.WriteString(`<ul class="response-faces">` + faces.String() + `</ul>`)
	}
	if replies.Len() > 0 {
		b.WriteString(`<ul class="response-replies">` + replies.String() + `</ul>`)
	}
	b.WriteString(`</section>`)
	return b.String()
}

func mentionFace(x webmention.Mention) string {
	name := html.EscapeString(x.Author.Name)
	inner := name
	if x.Author.Photo != "" {
		inner = fmt.Sprintf(`<img class="u-photo" src="%s" alt="%s" loading="lazy">`,
			html.EscapeString(x.Author.Photo), name)
	}
	link := x.Author.URL
	if link == "" {
		link = x.URL
	}
	return fmt.Sprintf(
		`<li class="response %s h-cite"><a class="p-author h-card u-url" href="%s" title="%s">%s</a></li>`,
		html.EscapeString(x.Type), html.EscapeString(link), name, inner)
}

func mentionReply(x webmention.Mention) string {
	var b strings.Builder
	b.WriteString(`<li class="response reply h-cite">`)
	b.WriteString(`<a class="p-author h-card u-url" href="` + html.EscapeString(authorLink(x)) + `">`)
	if x.Author.Photo != "" {
		b.WriteString(`<img class="u-photo" src="` + html.EscapeString(x.Author.Photo) + `" alt="" loading="lazy">`)
	}
	b.WriteString(`<span class="p-name">` + html.EscapeString(x.Author.Name) + `</span></a>`)
	if x.Content != "" {
		b.WriteString(`<div class="p-content">` + html.EscapeString(x.Content) + `</div>`)
	}
	if x.URL != "" {
		b.WriteString(`<a class="u-url response-perma" href="` + html.EscapeString(x.URL) + `">`)
		if x.Published != "" {
			b.WriteString(`<time class="dt-published">` + html.EscapeString(x.Published) + `</time>`)
		} else {
			b.WriteString(`permalink`)
		}
		b.WriteString(`</a>`)
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

func mentionsAssetJSON(m webmention.Mentions) ([]byte, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
