package build

import (
	_ "embed"
	"html"
	"strings"

	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/telemetry"
)

// analyticsJS is the colophon-owned web beacon, written once to the output root when web
// analytics is enabled. It is theme-agnostic (referenced by every theme via analytics_head),
// so it lives here rather than being duplicated per theme.
//
//go:embed assets/analytics.js
var analyticsJS []byte

// analyticsAsset is the output path of the beacon, relative to the site root.
const analyticsAsset = "analytics.js"

// analyticsHead returns the <script> tag that loads the beacon for one page, carrying the
// statsfactory endpoint/key and the page's public dimensions as data-* attributes. It returns
// "" when web analytics is disabled. p is nil for non-post pages (index, tag, author), which
// then carry no post dimensions. The hidden persona is deliberately never emitted here.
func analyticsHead(site core.Site, basePath string, p *page) string {
	if !site.Analytics.WebEnabled() {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<script defer src="`)
	b.WriteString(html.EscapeString(basePath + analyticsAsset))
	b.WriteString(`"`)
	attr := func(name, val string) {
		if val == "" {
			return
		}
		b.WriteString(" ")
		b.WriteString(name)
		b.WriteString(`="`)
		b.WriteString(html.EscapeString(val))
		b.WriteString(`"`)
	}
	attr("data-sf-url", strings.TrimRight(site.Analytics.ServerURL, "/"))
	attr("data-sf-key", site.Analytics.AppKey)
	if p != nil {
		attr("data-sf-slug", slugFromURL(p.URL))
		attr("data-sf-type", p.Type)
		attr("data-sf-author", p.Author)
		attr("data-sf-tags", strings.Join(p.Tags, ","))
	}
	b.WriteString(`></script>`)
	return b.String()
}

// emitBuildTelemetry sends the server-side build events: the overall build, the document
// count per source (by driver type), and the post count per persona voice. It is a no-op when
// t is nil (e.g. a serve preview rebuild) or disabled, so preview builds never emit anything.
func emitBuildTelemetry(t *telemetry.Client, site core.Site, docs []sourceDoc, pages []page) {
	if t == nil {
		return
	}
	t.Build(site.Theme, len(pages))

	srcCount := map[string]int{}
	srcDriver := map[string]string{}
	for _, d := range docs {
		id := d.src.ID()
		srcCount[id]++
		srcDriver[id] = d.src.Driver()
	}
	for id, n := range srcCount {
		t.Source(srcDriver[id], id, n)
	}

	personaCount := map[string]int{}
	for _, p := range pages {
		if p.Static {
			continue
		}
		persona := p.Persona
		if persona == "" {
			persona = "(none)"
		}
		personaCount[persona]++
	}
	for persona, n := range personaCount {
		t.Persona(persona, n)
	}
}

// slugFromURL extracts the trailing path segment of a base_path-relative URL ("posts/hello/"
// → "hello"), the post's slug, for the post.slug dimension.
func slugFromURL(url string) string {
	trimmed := strings.Trim(url, "/")
	if i := strings.LastIndexByte(trimmed, '/'); i >= 0 {
		return trimmed[i+1:]
	}
	return trimmed
}
