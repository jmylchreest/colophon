package build

import (
	"bytes"
	"html"
	"path"
	"regexp"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	ghtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// sharedMarkdown is the process-wide goldmark renderer. A goldmark.Markdown is immutable
// once built and its Convert is safe for concurrent use, so it is constructed once instead
// of per build — every Extend (registering parser/renderer pairs) runs a single time.
var sharedMarkdown = newMarkdown()

// newMarkdown builds the shared goldmark renderer. colophon emits progressive-enhancement
// HTML: special blocks become semantic elements that carry their raw source as text and
// are tagged by type, so a no-JS theme shows readable raw text while a theme that knows
// the type enhances it (highlight.js for code, KaTeX for math, Mermaid for diagrams). The
// build depends on no JS library; it only guarantees the markup contract. Unsafe HTML is
// allowed because callout preprocessing emits <div> wrappers and blog content is trusted.
func newMarkdown() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(extension.GFM, mathExtension{}, mediaExtension{}),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(
			ghtml.WithUnsafe(),
			renderer.WithNodeRenderers(util.Prioritized(codeRenderer{}, 10), util.Prioritized(tableRenderer{}, 10)),
		),
	)
}

// --- inline video/audio ---
//
// A markdown image embed whose destination is a video or audio file — ![caption](demo.mp4),
// or Obsidian ![[demo.mp4]] which the source normalises to that form — is rendered as a
// <video>/<audio> player instead of a broken <img>. Discovery, copying and R2 routing already
// treat any image-syntax destination uniformly, so the file ships and its URL is rewritten
// with no extra wiring; only the rendered element changes. Destinations are matched by
// extension, so a direct external file URL (…/clip.mp4) plays too.
var videoMIME = map[string]string{
	".mp4": "video/mp4", ".webm": "video/webm", ".mov": "video/quicktime",
	".m4v": "video/x-m4v", ".ogv": "video/ogg",
}

var audioMIMEInline = map[string]string{
	".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".aac": "audio/aac", ".oga": "audio/ogg",
	".ogg": "audio/ogg", ".wav": "audio/wav", ".flac": "audio/flac", ".opus": "audio/opus",
}

// mediaExt returns the lower-cased file extension of a URL, ignoring any ?query/#fragment.
func mediaExt(rawURL string) string {
	s := rawURL
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(path.Ext(s))
}

// mediaKind reports "video", "audio", or "" (a normal image) for an embed destination.
func mediaKind(dest string) string {
	ext := mediaExt(dest)
	if _, ok := videoMIME[ext]; ok {
		return "video"
	}
	if _, ok := audioMIMEInline[ext]; ok {
		return "audio"
	}
	return ""
}

func mediaMIME(dest string) string {
	ext := mediaExt(dest)
	if m, ok := videoMIME[ext]; ok {
		return m
	}
	return audioMIMEInline[ext]
}

var kindMedia = ast.NewNodeKind("Media")

// mediaNode replaces an image node that points at a video/audio file.
type mediaNode struct {
	ast.BaseInline
	media string // "video" | "audio"
	dest  string
	alt   string
}

func (n *mediaNode) Kind() ast.NodeKind         { return kindMedia }
func (n *mediaNode) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

type mediaExtension struct{}

func (mediaExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithASTTransformers(util.Prioritized(mediaTransformer{}, 500)))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(mediaRenderer{}, 10)))
}

// mediaTransformer swaps image nodes whose destination is a media file for mediaNodes, leaving
// every other image to goldmark's default <img> rendering. Matches are collected first, then
// replaced, so editing the tree does not disturb the in-progress walk.
type mediaTransformer struct{}

func (mediaTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	source := reader.Source()
	var imgs []*ast.Image
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if img, ok := n.(*ast.Image); ok && mediaKind(string(img.Destination)) != "" {
			imgs = append(imgs, img)
		}
		return ast.WalkContinue, nil
	})
	for _, img := range imgs {
		parent := img.Parent()
		if parent == nil {
			continue
		}
		parent.ReplaceChild(parent, img, &mediaNode{
			media: mediaKind(string(img.Destination)),
			dest:  string(img.Destination),
			alt:   altText(img, source),
		})
	}
}

// altText concatenates the literal text children of an image node (its alt text / caption).
func altText(n ast.Node, source []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			b.Write(t.Segment.Value(source))
		}
	}
	return b.String()
}

type mediaRenderer struct{}

func (mediaRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindMedia, renderMediaNode)
}

func renderMediaNode(w util.BufWriter, _ []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*mediaNode)
	src := html.EscapeString(n.dest)
	mime := mediaMIME(n.dest)
	if n.media == "video" {
		_, _ = w.WriteString(`<video class="post-video" controls preload="metadata" playsinline`)
		if n.alt != "" {
			_, _ = w.WriteString(` aria-label="` + html.EscapeString(n.alt) + `"`)
		}
		_, _ = w.WriteString(`><source src="` + src + `"`)
		if mime != "" {
			_, _ = w.WriteString(` type="` + mime + `"`)
		}
		_, _ = w.WriteString("></video>")
		return ast.WalkSkipChildren, nil
	}
	_, _ = w.WriteString(`<audio class="post-inline-audio" controls preload="metadata" src="` + src + `"`)
	if n.alt != "" {
		_, _ = w.WriteString(` aria-label="` + html.EscapeString(n.alt) + `"`)
	}
	_, _ = w.WriteString("></audio>")
	return ast.WalkSkipChildren, nil
}

// --- code & mermaid blocks ---

// codeRenderer overrides the default code-block rendering so a ```mermaid fence emits the
// element Mermaid enhances (<pre class="mermaid">), and every other fence keeps the
// language-tagged <pre><code class="language-x"> highlight.js/Prism expect.
type codeRenderer struct{}

func (codeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindFencedCodeBlock, renderCode)
	reg.Register(ast.KindCodeBlock, renderCode)
}

func renderCode(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	var (
		lines *text.Segments
		lang  []byte
	)
	switch n := node.(type) {
	case *ast.FencedCodeBlock:
		lines, lang = n.Lines(), n.Language(source)
	case *ast.CodeBlock:
		lines = n.Lines()
	}
	var raw bytes.Buffer
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		raw.Write(seg.Value(source))
	}

	if string(lang) == "mermaid" {
		_, _ = w.WriteString(`<pre tabindex="0" class="mermaid">`)
		_, _ = w.WriteString(html.EscapeString(raw.String()))
		_, _ = w.WriteString("</pre>\n")
		return ast.WalkSkipChildren, nil
	}

	// tabindex="0" makes the (horizontally scrollable) code block keyboard-focusable, so a
	// keyboard-only user can scroll it — WCAG 2.1.1 (axe scrollable-region-focusable). A bare
	// tabindex is the right fix here: an aria-label on a <pre> (generic role) is prohibited (4.1.2).
	_, _ = w.WriteString(`<pre tabindex="0"><code`)
	if len(lang) > 0 {
		_, _ = w.WriteString(` class="language-`)
		_, _ = w.WriteString(html.EscapeString(string(lang)))
		_, _ = w.WriteString(`"`)
	}
	_, _ = w.WriteString(">")
	_, _ = w.WriteString(html.EscapeString(raw.String()))
	_, _ = w.WriteString("</code></pre>\n")
	return ast.WalkSkipChildren, nil
}

// --- tables ---

// tableRenderer wraps a GFM table in a focusable, horizontally-scrollable container, so a
// keyboard-only user can scroll a wide table — WCAG 2.1.1. Only the <table> element is overridden;
// goldmark's header/row/cell renderers (which manage <thead>/<tbody> and column alignment) are
// left untouched, so the table renders exactly as before, just inside the wrapper.
type tableRenderer struct{}

func (tableRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(extast.KindTable, renderTableWrapped)
}

func renderTableWrapped(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<div class="table-scroll" tabindex="0">` + "\n<table>\n")
	} else {
		_, _ = w.WriteString("</table>\n</div>\n")
	}
	return ast.WalkContinue, nil
}

// --- math ---

var kindMath = ast.NewNodeKind("Math")

// mathNode carries raw LaTeX (inline $…$ or display $$…$$) verbatim, untouched by markdown
// emphasis/escaping, for a theme to typeset with KaTeX (or show as-is without JS).
type mathNode struct {
	ast.BaseInline
	Display bool
	Value   []byte
}

func (n *mathNode) Kind() ast.NodeKind         { return kindMath }
func (n *mathNode) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

type mathExtension struct{}

func (mathExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(parser.WithInlineParsers(util.Prioritized(mathParser{}, 100)))
	m.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(mathRenderer{}, 10)))
}

type mathParser struct{}

func (mathParser) Trigger() []byte { return []byte{'$'} }

// Parse matches single-line inline $…$ and display $$…$$. A currency heuristic (opening $
// not followed by a space/digit, closing $ not preceded by a space nor followed by a
// digit) keeps "$5 and $10" from being read as math.
func (mathParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	if len(line) < 2 || line[0] != '$' {
		return nil
	}
	display := line[1] == '$'
	delim := 1
	if display {
		delim = 2
	}
	rest := line[delim:]
	end := -1
	if display {
		end = bytes.Index(rest, []byte("$$"))
	} else {
		if rest[0] == ' ' || rest[0] == '\t' || isDigit(rest[0]) {
			return nil
		}
		for i := 0; i < len(rest); i++ {
			if rest[i] != '$' {
				continue
			}
			if i > 0 && (rest[i-1] == ' ' || rest[i-1] == '\t') {
				continue
			}
			if i+1 < len(rest) && isDigit(rest[i+1]) {
				continue
			}
			end = i
			break
		}
	}
	if end < 0 {
		return nil
	}
	value := rest[:end]
	if len(bytes.TrimSpace(value)) == 0 {
		return nil
	}
	block.Advance(delim + end + delim)
	return &mathNode{Display: display, Value: value}
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

type mathRenderer struct{}

func (mathRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindMath, renderMath)
}

func renderMath(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*mathNode)
	if n.Display {
		// Display math can overflow horizontally (the theme scrolls it), so make it
		// keyboard-focusable — WCAG 2.1.1. Inline math doesn't scroll, so it stays a span.
		_, _ = w.WriteString(`<div tabindex="0" class="math math-display">`)
		_, _ = w.WriteString(html.EscapeString(string(n.Value)))
		_, _ = w.WriteString("</div>")
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString(`<span class="math math-inline">`)
	_, _ = w.WriteString(html.EscapeString(string(n.Value)))
	_, _ = w.WriteString("</span>")
	return ast.WalkContinue, nil
}

// --- callouts ---

var calloutRE = regexp.MustCompile(`^\[!(\w+)\]([+-]?)\s*(.*)$`)

// preprocessCallouts rewrites Obsidian callouts — a blockquote whose first line is
// `[!type] Title` — into <div class="callout callout-type"> wrappers (styled by CSS, no
// JS), leaving plain blockquotes untouched. It runs before goldmark; the body between the
// wrappers is left as markdown for goldmark to parse, so blank lines isolate the raw divs.
func preprocessCallouts(body string) string {
	lines := strings.Split(body, "\n")
	var out []string
	for i := 0; i < len(lines); {
		if !isQuote(lines[i]) {
			out = append(out, lines[i])
			i++
			continue
		}
		j := i
		var quoted []string
		for j < len(lines) && isQuote(lines[j]) {
			quoted = append(quoted, dequote(lines[j]))
			j++
		}
		if m := calloutRE.FindStringSubmatch(quoted[0]); m != nil {
			typ := strings.ToLower(m[1])
			title := strings.TrimSpace(m[3])
			if typ == "quote" {
				// A pull-quote / epigraph: render as a semantic <figure><blockquote> with the
				// title (when given) as the attribution <figcaption> below it, themed large.
				out = append(out, "", `<figure class="pullquote">`, `<blockquote>`, "")
				out = append(out, quoted[1:]...)
				out = append(out, "", `</blockquote>`)
				if title != "" {
					out = append(out, `<figcaption>`+html.EscapeString(title)+`</figcaption>`)
				}
				out = append(out, `</figure>`, "")
			} else {
				if title == "" {
					title = capitalize(typ)
				}
				out = append(out, "",
					`<div class="callout callout-`+typ+`" data-callout="`+typ+`">`,
					`<div class="callout-title">`+html.EscapeString(title)+`</div>`,
					`<div class="callout-body">`, "")
				out = append(out, quoted[1:]...)
				out = append(out, "", "</div>", "</div>", "")
			}
		} else {
			out = append(out, lines[i:j]...)
		}
		i = j
	}
	return strings.Join(out, "\n")
}

func isQuote(line string) bool { return strings.HasPrefix(strings.TrimLeft(line, " "), ">") }

func dequote(line string) string {
	t := strings.TrimPrefix(strings.TrimLeft(line, " "), ">")
	return strings.TrimPrefix(t, " ")
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
