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

// resolvePersona returns the page's persona — the one named in frontmatter, else the first
// configured persona, else nil.
func resolvePersona(cfg *config.Config, id string) *core.Persona {
	for i := range cfg.Personas {
		if cfg.Personas[i].ID == id {
			return &cfg.Personas[i]
		}
	}
	if len(cfg.Personas) > 0 {
		return &cfg.Personas[0]
	}
	return nil
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
func seoHead(site core.Site, p page, persona *core.Persona) string {
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
	image := p.ImageAbs
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
	if !p.Published.IsZero() {
		iso := p.Published.UTC().Format(time.RFC3339)
		meta("property", "article:published_time", iso)
		meta("property", "article:modified_time", iso)
	}
	for _, t := range p.Tags {
		meta("property", "article:tag", t)
	}
	if a := personaName(persona); a != "" {
		meta("property", "article:author", a)
	}

	card := "summary"
	if image != "" {
		card = "summary_large_image"
	}
	meta("name", "twitter:card", card)
	meta("name", "twitter:title", ogTitle)
	meta("name", "twitter:description", ogDesc)
	meta("name", "twitter:image", image)

	b.WriteString(jsonLD(site, p, persona, canonical, desc, image, kw))
	return b.String()
}

// jsonLD renders the schema.org BlogPosting for a post. json.Marshal HTML-escapes <, > and
// &, so the result is safe to embed directly in a <script> element.
func jsonLD(site core.Site, p page, persona *core.Persona, canonical, desc, image, kw string) string {
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
	if persona != nil {
		atype := "Person"
		if persona.Kind == core.PersonaBrand {
			atype = "Organization"
		}
		author := map[string]any{"@type": atype, "name": personaName(persona)}
		if len(persona.HCard.URLs) > 0 {
			author["url"] = persona.HCard.URLs[0]
		}
		doc["author"] = author
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

func personaName(p *core.Persona) string {
	if p == nil {
		return ""
	}
	return firstNonEmpty(p.Byline, p.DisplayName)
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
