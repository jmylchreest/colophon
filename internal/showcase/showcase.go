// Package showcase provides a built-in markdown reference page, embedded in the binary, that
// demonstrates every content feature a theme can enrich. It is injected into a build as a
// synthetic document (see colophon serve --showcase) and never touches the user's content tree.
//
// To amend it, edit showcase.md and drop any referenced files into assets/ — both are embedded
// here, so a single edit ships with the binary. See the `showcase-coverage` decision: any new
// themeable engine output must be added here (and removed when deprecated).
package showcase

import (
	"context"
	"embed"
	"fmt"
	"io"
	"io/fs"

	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/markdown"
)

//go:embed showcase.md assets
var files embed.FS

// Slug is the URL slug the showcase renders at (/showcase/).
const Slug = "showcase"

// Glossary returns the showcase's own glossary terms, merged into the build's glossary only
// under --showcase, so the auto-decoration (pop-overs on first use) renders without the project
// shipping a glossary.yaml. The showcase prose mentions each of these.
func Glossary() map[string]core.GlossaryEntry {
	return map[string]core.GlossaryEntry{
		"TTS": {Definition: "Text To Speech — generating spoken audio from text."},
		// A multi-link entry: the decorator renders numbered superscripts (¹ ²) after the term.
		"IPA": {
			Definition: "International Phonetic Alphabet — a precise notation for pronunciation.",
			Links: []core.GlossaryLink{
				{Label: "IPA chart", URL: "https://www.internationalphoneticassociation.org/content/ipa-chart"},
				{Label: "Wikipedia", URL: "https://en.wikipedia.org/wiki/International_Phonetic_Alphabet"},
			},
		},
		"PCM": {Definition: "Pulse-Code Modulation — uncompressed digital audio samples."},
		// A single-link entry: the decorator renders one glyph (↗) after the term.
		"KaTeX": {
			Definition: "A fast math typesetting library that renders LaTeX in the browser.",
			Links:      []core.GlossaryLink{{Label: "KaTeX home", URL: "https://katex.org"}},
		},
	}
}

// Document parses the embedded showcase markdown into a content document plus a source that
// serves the embedded assets, ready to inject into a build.
func Document() (core.Content, core.Source, error) {
	raw, err := files.ReadFile("showcase.md")
	if err != nil {
		return core.Content{}, nil, fmt.Errorf("read embedded showcase: %w", err)
	}
	fm, body, err := markdown.ParseFrontmatter(raw)
	if err != nil {
		return core.Content{}, nil, fmt.Errorf("parse embedded showcase: %w", err)
	}
	c := core.Content{
		Document:   markdown.Document{Frontmatter: fm, Body: string(body)},
		SourcePath: Slug + ".md",
	}
	return c, source{}, nil
}

// source is a core.Source backed by the embedded files: it owns the synthetic showcase document
// and serves the placeholder assets it references (assets/placeholder.png, .wav, sample.txt).
type source struct{}

func (source) ID() string                                        { return "showcase" }
func (source) Driver() string                                    { return "embedded" }
func (source) Documents(context.Context) ([]core.Content, error) { return nil, nil }

func (source) Open(_ context.Context, ref string) (io.ReadCloser, error) {
	return files.Open(ref)
}

func (source) Resolve(_ context.Context, ref string) (string, bool) {
	if info, err := fs.Stat(files, ref); err == nil && !info.IsDir() {
		return ref, true
	}
	return "", false
}
