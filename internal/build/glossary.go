package build

import (
	_ "embed"
	"encoding/json"
	"html"

	"github.com/jmylchreest/colophon/internal/config"
)

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

// emitGlossary writes the glossary data + decorator to the site root when the project ships a
// glossary, and returns the per-page <script> markup (glossary_head) that loads them. The
// glossary itself is never rendered as a page; only the term→definition JSON is published, for
// the theme's decorator to wrap matching terms in <abbr title>. Returns "" when there is no
// glossary, so a theme's {{ glossary_head }} stays inert.
func emitGlossary(write func(string, []byte) error, cfg *config.Config, basePath string) (string, error) {
	if len(cfg.Glossary) == 0 {
		return "", nil
	}
	data, err := json.Marshal(cfg.Glossary)
	if err != nil {
		return "", err
	}
	if err := write(glossaryData, data); err != nil {
		return "", err
	}
	if err := write(glossaryAsset, glossaryJS); err != nil {
		return "", err
	}
	if err := write(glossaryStyle, glossaryCSS); err != nil {
		return "", err
	}
	return `<link rel="stylesheet" href="` + html.EscapeString(basePath+glossaryStyle) +
		`"><script defer src="` + html.EscapeString(basePath+glossaryAsset) +
		`" data-glossary="` + html.EscapeString(basePath+glossaryData) + `"></script>`, nil
}
