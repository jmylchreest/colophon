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

//go:embed assets/analytics-ga.js
var analyticsGAJS []byte

// Output paths of the per-provider client assets, relative to the site root.
const (
	analyticsAsset   = "analytics.js"
	analyticsGAAsset = "analytics-ga.js"
)

// emitAnalyticsAssets writes each enabled provider's client asset to the site root — and only
// that provider's. statsfactory ships its cookieless beacon; Google Analytics ships its gtag
// loader. Nothing is written when the master switch is off or no provider is configured.
func emitAnalyticsAssets(write func(string, []byte) error, master bool, a core.Analytics) error {
	if !master {
		return nil
	}
	if a.Statsfactory.Configured() {
		if err := write(analyticsAsset, analyticsJS); err != nil {
			return err
		}
	}
	if a.GoogleAnalytics.Configured() {
		if err := write(analyticsGAAsset, analyticsGAJS); err != nil {
			return err
		}
	}
	return nil
}

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
		b.WriteString(googleAnalyticsTag(a.GoogleAnalytics.MeasurementID, basePath))
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

// googleAnalyticsTag references the bundled GA loader (analytics-ga.js), passing the
// measurement id as a data attribute. The loader injects Google's gtag.js. Unlike the
// statsfactory beacon, GA sets cookies and carries its own consent obligations.
func googleAnalyticsTag(id, basePath string) string {
	return `<script defer src="` + html.EscapeString(basePath+analyticsGAAsset) +
		`" data-ga-id="` + html.EscapeString(id) + `"></script>`
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
