package build

import (
	"bytes"
	"html"
	"regexp"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
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
		goldmark.WithExtensions(extension.GFM, mathExtension{}),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(
			ghtml.WithUnsafe(),
			renderer.WithNodeRenderers(util.Prioritized(codeRenderer{}, 10)),
		),
	)
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
		_, _ = w.WriteString(`<pre class="mermaid">`)
		_, _ = w.WriteString(html.EscapeString(raw.String()))
		_, _ = w.WriteString("</pre>\n")
		return ast.WalkSkipChildren, nil
	}

	_, _ = w.WriteString("<pre><code")
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
	tag, class := "span", "math math-inline"
	if n.Display {
		tag, class = "div", "math math-display"
	}
	_, _ = w.WriteString("<" + tag + ` class="` + class + `">`)
	_, _ = w.WriteString(html.EscapeString(string(n.Value)))
	_, _ = w.WriteString("</" + tag + ">")
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
			if title == "" {
				title = capitalize(typ)
			}
			out = append(out, "",
				`<div class="callout callout-`+typ+`" data-callout="`+typ+`">`,
				`<div class="callout-title">`+html.EscapeString(title)+`</div>`,
				`<div class="callout-body">`, "")
			out = append(out, quoted[1:]...)
			out = append(out, "", "</div>", "</div>", "")
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
