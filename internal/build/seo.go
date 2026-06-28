package build

import (
	"encoding/json"
	"html"
	"strings"
	"time"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/markdown"
)

// resolveAuthor returns the page's byline author — the one named in frontmatter, else the
// first configured author, else "Anonymous". (Persona, the hidden voice, is resolved
// separately and only by the agent/corpus, never for rendering.)
func resolveAuthor(cfg *config.Config, id string) core.Author {
	if id != "" {
		if a := cfg.Author(id); a != nil {
			return *a
		}
	}
	if len(cfg.Authors) > 0 {
		return cfg.Authors[0]
	}
	return core.AnonymousAuthor()
}

// metaTitle is the <title>/og:title for a page: an explicit seo.title, else the page title.
func metaTitle(p page) string {
	if p.SEO != nil && p.SEO.Title != "" {
		return p.SEO.Title
	}
	return p.Title
}

// seoHead renders the SEO <head> block for a post: canonical, robots, description, Open
// Graph, Twitter card and a JSON-LD BlogPosting. Every value comes from the page's resolved
// fields and its seo: overrides, so the markup mirrors the frontmatter exactly.
func seoHead(site core.Site, p page, author core.Author) string {
	var s markdown.SEO
	if p.SEO != nil {
		s = *p.SEO
	}

	canonical := strings.TrimRight(site.BaseURL, "/") + "/" + p.URL
	if s.Canonical != "" {
		canonical = s.Canonical
	}
	desc := firstNonEmpty(s.Description, p.Description)
	ogTitle := firstNonEmpty(socialField(s, true), s.Title, p.Title)
	ogDesc := firstNonEmpty(socialField(s, false), s.Description, p.Description)
	// og:image prefers an explicit preview image (image:), then the hero cover art (hero:), so a
	// post that sets only a hero still gets a social preview; an absolute seo.image overrides both.
	image := firstNonEmpty(p.ImageAbs, p.HeroAbs)
	if isAbsURL(s.Image) {
		image = s.Image
	}
	noindex := s.NoIndex || p.Draft || p.Embargoed
	kw := seoKeywords(s, p)

	var b strings.Builder
	meta := func(kind, key, val string) {
		if val != "" {
			b.WriteString(`<meta ` + kind + `="` + key + `" content="` + html.EscapeString(val) + "\">\n")
		}
	}
	b.WriteString(`<link rel="canonical" href="` + html.EscapeString(canonical) + "\">\n")
	// hreflang alternates: tell search engines about this post's other-language versions, plus an
	// x-default pointing at the site's default language.
	for _, t := range p.Translations {
		b.WriteString(`<link rel="alternate" hreflang="` + html.EscapeString(t.Lang) + `" href="` + html.EscapeString(t.Abs) + "\">\n")
		if t.Default {
			b.WriteString(`<link rel="alternate" hreflang="x-default" href="` + html.EscapeString(t.Abs) + "\">\n")
		}
	}
	meta("name", "description", desc)
	if noindex {
		b.WriteString("<meta name=\"robots\" content=\"noindex,nofollow\">\n")
	}
	meta("name", "keywords", kw)

	meta("property", "og:type", "article")
	meta("property", "og:site_name", site.Title)
	meta("property", "og:url", canonical)
	meta("property", "og:title", ogTitle)
	meta("property", "og:description", ogDesc)
	meta("property", "og:image", image)
	meta("property", "og:locale", ogLocale(firstNonEmpty(p.Lang, site.Lang)))
	if !p.Published.IsZero() {
		iso := p.Published.UTC().Format(time.RFC3339)
		meta("property", "article:published_time", iso)
		meta("property", "article:modified_time", iso)
	}
	for _, t := range p.Tags {
		meta("property", "article:tag", t)
	}
	if author.Name != "" {
		meta("property", "article:author", author.Name)
	}

	card := "summary"
	if image != "" {
		card = "summary_large_image"
	}
	meta("name", "twitter:card", card)
	meta("name", "twitter:title", ogTitle)
	meta("name", "twitter:description", ogDesc)
	meta("name", "twitter:image", image)

	b.WriteString(jsonLD(site, p, author, canonical, desc, image, kw))
	return b.String()
}

// jsonLD renders the schema.org BlogPosting for a post. json.Marshal HTML-escapes <, > and
// &, so the result is safe to embed directly in a <script> element.
func jsonLD(site core.Site, p page, author core.Author, canonical, desc, image, kw string) string {
	typ := "BlogPosting"
	if p.SEO != nil && p.SEO.Type != "" {
		typ = p.SEO.Type
	}
	doc := map[string]any{
		"@context":         "https://schema.org",
		"@type":            typ,
		"headline":         metaTitle(p),
		"url":              canonical,
		"mainEntityOfPage": canonical,
	}
	if desc != "" {
		doc["description"] = desc
	}
	if image != "" {
		doc["image"] = image
	}
	if kw != "" {
		doc["keywords"] = kw
	}
	if !p.Published.IsZero() {
		iso := p.Published.UTC().Format(time.RFC3339)
		doc["datePublished"] = iso
		doc["dateModified"] = iso
	}
	if author.Name != "" {
		a := map[string]any{"@type": "Person", "name": author.Name}
		if len(author.URLs) > 0 {
			a["url"] = author.URLs[0]
		}
		doc["author"] = a
	}
	if site.Title != "" {
		doc["publisher"] = map[string]any{"@type": "Organization", "name": site.Title}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return ""
	}
	return `<script type="application/ld+json">` + string(out) + "</script>\n"
}

// socialField returns the seo.social title (titleField true) or description override.
func socialField(s markdown.SEO, titleField bool) string {
	if s.Social == nil {
		return ""
	}
	if titleField {
		return s.Social.Title
	}
	return s.Social.Description
}

// seoKeywords joins the explicit seo.keywords, else the page's tags.
func seoKeywords(s markdown.SEO, p page) string {
	if len(s.Keywords) > 0 {
		return strings.Join(s.Keywords, ", ")
	}
	return strings.Join(p.Tags, ", ")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func isAbsURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// ogLocale converts a BCP-47 language tag to the underscore form Open Graph expects
// (en-GB → en_GB). An empty tag defaults to "en".
func ogLocale(lang string) string {
	return strings.ReplaceAll(defaultLang(lang), "-", "_")
}

// listingSEOHead renders the SEO <head> block for a listing page — the home page, a tag index or
// an author index. Listings carry no per-post metadata, so the markup draws on the site's title,
// description and default share image. It emits canonical, description, website-flavoured Open
// Graph + Twitter, og:locale, and a schema.org JSON-LD: a Blog for the home page (home true), a
// CollectionPage for the narrower tag/author listings.
func listingSEOHead(site core.Site, canonical, title, desc, image string, home bool) string {
	var b strings.Builder
	meta := func(kind, key, val string) {
		if val != "" {
			b.WriteString(`<meta ` + kind + `="` + key + `" content="` + html.EscapeString(val) + "\">\n")
		}
	}
	b.WriteString(`<link rel="canonical" href="` + html.EscapeString(canonical) + "\">\n")
	meta("name", "description", desc)
	meta("property", "og:type", "website")
	meta("property", "og:site_name", site.Title)
	meta("property", "og:url", canonical)
	meta("property", "og:title", title)
	meta("property", "og:description", desc)
	meta("property", "og:image", image)
	meta("property", "og:locale", ogLocale(site.Lang))

	card := "summary"
	if image != "" {
		card = "summary_large_image"
	}
	meta("name", "twitter:card", card)
	meta("name", "twitter:title", title)
	meta("name", "twitter:description", desc)
	meta("name", "twitter:image", image)

	typ := "CollectionPage"
	if home {
		typ = "Blog"
	}
	doc := map[string]any{
		"@context": "https://schema.org",
		"@type":    typ,
		"name":     title,
		"url":      canonical,
	}
	if desc != "" {
		doc["description"] = desc
	}
	if image != "" {
		doc["image"] = image
	}
	if site.Title != "" {
		doc["publisher"] = map[string]any{"@type": "Organization", "name": site.Title}
	}
	if out, err := json.Marshal(doc); err == nil {
		b.WriteString(`<script type="application/ld+json">` + string(out) + "</script>\n")
	}
	return b.String()
}
