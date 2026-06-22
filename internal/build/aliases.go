package build

import (
	"html"
	"sort"
	"strings"

	"github.com/jmylchreest/colophon/internal/clog"
)

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
// An alias that collides with a real page is skipped with a warning (the real page wins).
func emitRedirects(write func(string, []byte) error, pages []page, basePath, baseURL string, log *clog.Logger) error {
	// Real output paths, so an alias never clobbers a page.
	taken := map[string]bool{}
	for _, p := range pages {
		taken[p.URL] = true // base_path-relative, e.g. "posts/hello/"
	}

	var rules []string // "<from> <to> 301" lines for _redirects
	seen := map[string]bool{}
	for _, p := range pages {
		target := basePath + p.URL // root-relative, e.g. "/posts/hello/"
		canonical := absURL(baseURL, p.URL)
		for _, a := range p.Aliases {
			urlPath := a + "/" // mirror page URL shape ("old/")
			if taken[urlPath] {
				log.Step("ALIAS", p.Slug, "skipped", a, "reason", "collides with a page")
				continue
			}
			if seen[urlPath] {
				log.Step("ALIAS", p.Slug, "skipped", a, "reason", "duplicate alias")
				continue
			}
			seen[urlPath] = true
			if err := write(a+"/index.html", redirectStub(canonical, target)); err != nil {
				return err
			}
			rules = append(rules, basePath+urlPath+" "+target+" 301")
		}
	}

	// .nojekyll is unconditional and harmless everywhere; only GitHub Pages needs it (without it,
	// Jekyll strips the _search/ and _mentions/ directories and the raw redirect stubs).
	if err := write(".nojekyll", nil); err != nil {
		return err
	}
	if len(rules) > 0 {
		sort.Strings(rules)
		if err := write("_redirects", []byte(strings.Join(rules, "\n")+"\n")); err != nil {
			return err
		}
		log.Detail("BUILD", "", "redirects", len(rules))
	}
	return nil
}
