package build

import (
	_ "embed"
	"encoding/json"
	"html"
	"regexp"
	"sort"
	"strings"

	"github.com/jmylchreest/colophon/internal/config"
)

// glossaryMatcher compiles a case-insensitive, word-boundary regex over all glossary terms,
// or nil when the glossary is empty. It is used to decide which pages actually reference a
// term, so the decorator + assets ship only where they're needed.
func glossaryMatcher(gloss map[string]string) *regexp.Regexp {
	if len(gloss) == 0 {
		return nil
	}
	terms := make([]string, 0, len(gloss))
	for t := range gloss {
		terms = append(terms, regexp.QuoteMeta(t))
	}
	// Longest first keeps a multi-word term from being masked by a contained shorter one.
	sort.Slice(terms, func(i, j int) bool { return len(terms[i]) > len(terms[j]) })
	return regexp.MustCompile(`(?i)\b(` + strings.Join(terms, "|") + `)\b`)
}

// pageNeedsGlossary reports whether a rendered page actually uses the glossary: a term appears
// in its HTML (auto-matchable), or — when the post opted out of auto-matching — it carries an
// explicit <dfn> force. A page with no terms never loads the decorator.
func pageNeedsGlossary(htmlBody string, optedOut bool, re *regexp.Regexp) bool {
	if re == nil || !re.MatchString(htmlBody) {
		return false
	}
	if optedOut {
		return strings.Contains(htmlBody, "<dfn")
	}
	return true
}

//go:embed assets/glossary.js
var glossaryJS []byte

//go:embed assets/glossary.css
var glossaryCSS []byte

// Output paths of the glossary data + engine-provided decorator/styles, relative to the site
// root. The CSS is engine-owned so every theme gets a working popover without copying styles;
// a theme can still override .gloss/.gloss-tip.
const (
	glossaryData  = "glossary.json"
	glossaryAsset = "glossary.js"
	glossaryStyle = "glossary.css"
)

// emitGlossary writes the glossary data + engine decorator/styles to the site root when the
// project ships a glossary, and reports whether it did. The glossary itself is never rendered
// as a page; only the term→definition JSON is published. Per-page markup is built by
// glossaryHeadTag, so a post can carry the auto-match flag.
func emitGlossary(write func(string, []byte) error, cfg *config.Config) (bool, error) {
	if len(cfg.Glossary) == 0 {
		return false, nil
	}
	data, err := json.Marshal(cfg.Glossary)
	if err != nil {
		return false, err
	}
	for _, a := range []struct {
		name string
		body []byte
	}{{glossaryData, data}, {glossaryAsset, glossaryJS}, {glossaryStyle, glossaryCSS}} {
		if err := write(a.name, a.body); err != nil {
			return false, err
		}
	}
	return true, nil
}

// glossaryHeadTag is the per-page markup that loads the glossary styles + decorator. auto is
// false for a post that sets `glossary: false`: the decorator still loads (so an explicit
// <dfn> can force a single term) but skips automatic matching.
func glossaryHeadTag(basePath string, auto bool) string {
	tag := `<link rel="stylesheet" href="` + html.EscapeString(basePath+glossaryStyle) +
		`"><script defer src="` + html.EscapeString(basePath+glossaryAsset) +
		`" data-glossary="` + html.EscapeString(basePath+glossaryData) + `"`
	if !auto {
		tag += ` data-gloss-auto="off"`
	}
	return tag + `></script>`
}
