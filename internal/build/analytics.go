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

// analyticsHead returns the per-page analytics markup for one page: a statsfactory beacon
// script and/or a Google Analytics tag, for whichever providers the site configures. It
// returns "" when the telemetry master switch is off or no provider is configured. p is nil
// for non-post pages (index, tag, author), which carry no post dimensions. The hidden persona
// is deliberately never emitted here. master is the top-level telemetry switch.
func analyticsHead(master bool, a core.Analytics, basePath string, p *page) string {
	if !master {
		return ""
	}
	var b strings.Builder
	if a.Statsfactory.Configured() {
		b.WriteString(statsfactoryBeacon(a.Statsfactory, basePath, p))
	}
	if a.GoogleAnalytics.Configured() {
		b.WriteString(googleAnalyticsTag(a.GoogleAnalytics.MeasurementID))
	}
	return b.String()
}

// statsfactoryBeacon is the <script> tag that loads the cookieless beacon for one page,
// carrying the endpoint/key and the page's public dimensions as data-* attributes.
func statsfactoryBeacon(cfg core.AnalyticsStatsfactory, basePath string, p *page) string {
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
	attr("data-sf-url", strings.TrimRight(cfg.ServerURL, "/"))
	attr("data-sf-key", cfg.AppKey)
	if p != nil {
		attr("data-sf-slug", slugFromURL(p.URL))
		attr("data-sf-type", p.Type)
		attr("data-sf-author", p.Author)
		attr("data-sf-tags", strings.Join(p.Tags, ","))
	}
	b.WriteString(`></script>`)
	return b.String()
}

// googleAnalyticsTag is the standard GA4 gtag.js snippet. Unlike the statsfactory beacon, GA
// sets cookies and carries its own consent obligations — that is the site owner's call.
func googleAnalyticsTag(id string) string {
	id = html.EscapeString(id)
	return `<script async src="https://www.googletagmanager.com/gtag/js?id=` + id + `"></script>` +
		`<script>window.dataLayer=window.dataLayer||[];function gtag(){dataLayer.push(arguments);}` +
		`gtag('js',new Date());gtag('config','` + id + `');</script>`
}

// emitBuildTelemetry sends colophon's own build events — the overall build and the document
// count per source driver type — to the tool-telemetry client. It is a no-op when t is nil
// (e.g. a serve preview rebuild) or disabled, so previews never emit anything. The hidden
// persona is intentionally NOT reported here: it is site-owner content, not tool usage.
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
