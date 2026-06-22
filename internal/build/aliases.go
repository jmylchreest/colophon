package build

import (
	"html"
	"sort"
	"strings"

	"github.com/jmylchreest/colophon/internal/clog"
	"github.com/jmylchreest/colophon/internal/config"
)

// AliasConflict is an alias that can't be honoured cleanly: it either matches a real page (Page
// set) or is claimed by more than one post (Posts has the contenders). The build still resolves
// these deterministically (page wins; otherwise oldest wins) — this surfaces them for doctor.
type AliasConflict struct {
	Alias string   // normalised alias path, e.g. "old-name"
	Page  string   // slug of the page it collides with, or ""
	Posts []string // slugs claiming this alias when ≥2 (sorted), else nil
}

// AliasConflicts dry-resolves every declared alias against the gathered content and returns the
// conflicts, for `colophon doctor` preflight. It reads no bytes beyond the documents.
func AliasConflicts(cfg *config.Config) ([]AliasConflict, error) {
	docs, err := gatherDocuments(cfg, clog.Discard())
	if err != nil {
		return nil, err
	}
	pageSlug := map[string]bool{}
	for _, sd := range docs {
		pageSlug[slugFor(sd.doc.SourcePath, sd.doc.Frontmatter.Slug)] = true
	}
	owners := map[string][]string{} // alias path -> claiming slugs
	for _, sd := range docs {
		slug := slugFor(sd.doc.SourcePath, sd.doc.Frontmatter.Slug)
		for _, a := range normalizeAliases(sd.doc.Frontmatter.Aliases) {
			owners[a] = append(owners[a], slug)
		}
	}
	var out []AliasConflict
	for alias, claimers := range owners {
		c := AliasConflict{Alias: alias}
		hit := false
		if pageSlug[alias] {
			c.Page = alias
			hit = true
		}
		if len(claimers) > 1 {
			sort.Strings(claimers)
			c.Posts = claimers
			hit = true
		}
		if hit {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Alias < out[j].Alias })
	return out, nil
}

// normalizeAliases slugifies each declared alias path (same rules as a slug) and drops empties,
// so `aliases: [Old Name, 2020/old-post]` becomes ["old-name", "2020/old-post"].
func normalizeAliases(raw []string) []string {
	var out []string
	for _, a := range raw {
		if s := normalizeSlug(a); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// redirectStub is the portable, host-agnostic redirect: a tiny HTML page that points at the
// target. It works on every static host (the others — _redirects, S3 object metadata — are
// upgrades layered on top). canonical is absolute (search engines consolidate to it); the
// refresh/href is root-relative so it survives a domain change.
func redirectStub(canonical, rootRel string) []byte {
	c, r := html.EscapeString(canonical), html.EscapeString(rootRel)
	return []byte(`<!doctype html><html lang="en"><head><meta charset="utf-8">` +
		`<title>Redirecting…</title><link rel="canonical" href="` + c + `">` +
		`<meta name="robots" content="noindex">` +
		`<meta http-equiv="refresh" content="0; url=` + r + `">` +
		`<script>location.replace(` + jsString(r) + `)</script></head>` +
		`<body>Redirecting to <a href="` + r + `">` + r + `</a>.</body></html>` + "\n")
}

// jsString quotes s as a JS string literal for the location.replace argument (the value is the
// already-HTML-escaped root-relative path; <  is further escaped to avoid a </script> breakout).
func jsString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '<':
			b.WriteString("\\u003c")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// emitRedirects writes the three host-agnostic redirect artifacts for the build:
//   - a meta-refresh stub at <alias>/index.html → the target post, for every alias;
//   - a `_redirects` file (Cloudflare Pages / Netlify / GitLab Pages format) when any alias exists;
//   - `.nojekyll` always, so GitHub Pages serves the stubs and the _search/ _mentions/ dirs.
//
// Collisions resolve deterministically with a warning: an alias matching a real page is dropped
// (the page wins), and when two posts claim the same alias the OLDEST page wins (it likely
// established the URL first — inbound links/webmentions point at it), tie-broken by slug.
func emitRedirects(write func(string, []byte) error, pages []page, basePath, baseURL string, log *clog.Logger) error {
	// Real output paths, so an alias never clobbers a page.
	taken := map[string]bool{}
	for _, p := range pages {
		taken[p.URL] = true // base_path-relative, e.g. "posts/hello/"
	}

	// Resolve a single winner per alias path (oldest wins).
	winner := map[string]int{} // alias URL path ("old/") -> page index
	for i := range pages {
		for _, a := range pages[i].Aliases {
			up := a + "/"
			if taken[up] {
				log.Step("ALIAS", pages[i].Slug, "skipped", a, "reason", "collides with a page")
				continue
			}
			if w, ok := winner[up]; ok {
				keep, lose := w, i
				if olderPage(pages[i], pages[w]) {
					keep, lose = i, w
					winner[up] = i
				}
				log.Step("ALIAS", pages[lose].Slug, "skipped", a, "reason", "duplicate alias (kept "+pages[keep].Slug+")")
				continue
			}
			winner[up] = i
		}
	}

	ups := make([]string, 0, len(winner))
	for up := range winner {
		ups = append(ups, up)
	}
	sort.Strings(ups)

	var rules []string
	for _, up := range ups {
		p := pages[winner[up]]
		target := basePath + p.URL // root-relative, e.g. "/posts/hello/"
		if err := write(up+"index.html", redirectStub(absURL(baseURL, p.URL), target)); err != nil {
			return err
		}
		rules = append(rules, basePath+up+" "+target+" 301")
	}

	// .nojekyll is unconditional and harmless everywhere; only GitHub Pages needs it (without it,
	// Jekyll strips the _search/ and _mentions/ directories and the raw redirect stubs).
	if err := write(".nojekyll", nil); err != nil {
		return err
	}
	if len(rules) > 0 {
		if err := write("_redirects", []byte(strings.Join(rules, "\n")+"\n")); err != nil {
			return err
		}
		log.Detail("BUILD", "", "redirects", len(rules))
	}
	return nil
}

// olderPage reports whether x predates y: earlier Published wins, slug breaks ties so the result
// is deterministic regardless of page order.
func olderPage(x, y page) bool {
	if !x.Published.Equal(y.Published) {
		return x.Published.Before(y.Published)
	}
	return x.Slug < y.Slug
}
