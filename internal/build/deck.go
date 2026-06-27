package build

// Derive a self-contained slide deck from a post's rendered HTML. See slide.md. The body is split
// into sections at the configured boundaries (default every heading / <hr> / <splitslide>); a
// boundary heading is the slide title and deeper headings fold into bullets. Each section's blocks
// (prose, code, tables, figures, math, diagrams, callouts) are PAGINATED onto slides: blocks are
// packed until the slide is full, then spill to a continuation slide; an oversized code block gets
// its own slide and is truncated (with a link back to the post) if it still won't fit; images/video
// scale to fit. A cover slide (title/description/author) leads. Author escape hatches: <slide>…
// </slide> is one verbatim slide, <splitslide> forces a break, <noslide>…</noslide> is dropped. The
// deck is one HTML file with CSS + reader JS inlined; KaTeX/Mermaid/highlight hydrate from the same
// /vendor assets the theme ships. Degrades without JS to a readable stacked document.

import (
	"bytes"
	"html"
	"regexp"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var (
	deckHeadingRE   = regexp.MustCompile(`(?is)<h[1-6][^>]*>(.*?)</h[1-6]>`)
	deckDividerRE   = regexp.MustCompile(`(?is)^\s*(?:<hr\s*/?>|<splitslide\s*/?>(?:\s*</splitslide>)?)`)
	deckNoSlideRE   = regexp.MustCompile(`(?is)<noslide>.*?</noslide>`)
	deckSlideWrapRE = regexp.MustCompile(`(?is)<slide>(.*?)</slide>`)
	deckTitleRE     = regexp.MustCompile(`(?is)^\s*<h[1-6][^>]*>(.*?)</h[1-6]>`)
	deckCodeRE      = regexp.MustCompile(`(?is)(<pre[^>]*>\s*<code[^>]*>)(.*?)(</code>\s*</pre>)`)
)

// Pagination tuning: a slide's content budget in estimated "lines", and the hard cap past which a
// single block (a long code listing) is truncated rather than given an oversized own-slide.
const (
	deckBudget   = 19
	deckHardCap  = 32
	deckCharsPer = 58 // approx characters per line at the slide body font size
)

// deckMeta carries the post-level facts the deck needs beyond its body: the cover details, the link
// back to the post (Esc target + truncated-code link), and the base path for the /vendor hydration.
type deckMeta struct {
	Title       string
	Description string
	Author      string
	Avatar      string
	Date        string
	PostURL     string
	BasePath    string
}

// DefaultDeckSplit is the slide-boundary list when a post sets no `slides.split`: a new slide before
// each heading (any level), each <hr>, and each explicit <splitslide>. Narrow it (e.g. [h2]) to make
// deeper headings fold into bullets instead of starting their own slide.
var DefaultDeckSplit = []string{"h1", "h2", "h3", "h4", "h5", "h6", "hr", "splitslide"}

// deckBoundaryRE compiles the split targets into one boundary regex. Targets: h1–h6, hr, splitslide;
// the block kinds image, table, code, math, diagram (mermaid), audio, video; and text:<match> (split
// before a block whose text begins with <match>). Unknown targets are ignored; empty falls back to h2.
func deckBoundaryRE(split []string) *regexp.Regexp {
	if len(split) == 0 {
		split = DefaultDeckSplit
	}
	var parts []string
	for _, raw := range split {
		t := strings.ToLower(strings.TrimSpace(raw))
		switch {
		case len(t) == 2 && t[0] == 'h' && t[1] >= '1' && t[1] <= '6':
			parts = append(parts, `<`+t+`[\s>]`)
		case t == "hr":
			parts = append(parts, `<hr\b`)
		case t == "splitslide":
			parts = append(parts, `<splitslide\b`)
		case t == "image":
			parts = append(parts, `<figure\b|<img\b`)
		case t == "table":
			parts = append(parts, `<div[^>]*class="table-scroll"`)
		case t == "code":
			parts = append(parts, `<pre[^>]*><code`)
		case t == "math":
			parts = append(parts, `<div[^>]*math-display`)
		case t == "diagram" || t == "mermaid":
			parts = append(parts, `<pre[^>]*mermaid`)
		case t == "audio":
			parts = append(parts, `<audio\b`)
		case t == "video":
			parts = append(parts, `<video\b`)
		case strings.HasPrefix(t, "text:"):
			if m := strings.TrimSpace(strings.TrimSpace(raw)[5:]); m != "" {
				parts = append(parts, `<[a-z][a-z0-9]*[^>]*>\s*`+regexp.QuoteMeta(m))
			}
		}
	}
	if len(parts) == 0 {
		parts = []string{`<h2[\s>]`}
	}
	return regexp.MustCompile(`(?is)(` + strings.Join(parts, "|") + `)`)
}

// BuildDeck renders markdown to a single self-contained HTML slide deck. Used by the `colophon deck`
// CLI (one-shot); the build calls buildDeckHTML directly with the post's already-rendered HTML.
func BuildDeck(md, title string, split []string) (string, error) {
	var buf bytes.Buffer
	if err := sharedMarkdown.Convert([]byte(preprocessCallouts(md)), &buf); err != nil {
		return "", err
	}
	return buildDeckHTML(buf.String(), split, deckMeta{Title: title}), nil
}

// buildDeckHTML turns already-rendered post body HTML into the deck document. The body still carries
// the <slide>/<splitslide>/<noslide> markers; gen: images/glossary/math are already resolved.
func buildDeckHTML(body string, split []string, meta deckMeta) string {
	body = deckNoSlideRE.ReplaceAllString(body, "") // <noslide>…</noslide> never reaches the deck
	bound := deckBoundaryRE(split)

	slides := []string{coverSlide(meta)}
	// Walk the body, lifting <slide>…</slide> out as verbatim slides and paginating the runs between.
	last := 0
	for _, m := range deckSlideWrapRE.FindAllStringSubmatchIndex(body, -1) {
		slides = append(slides, sectionSlides(body[last:m[0]], bound, meta)...)
		slides = append(slides, explicitSlide(body[m[2]:m[3]]))
		last = m[1]
	}
	slides = append(slides, sectionSlides(body[last:], bound, meta)...)

	out := slides[:0]
	for _, s := range slides {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return deckDoc(meta, out, body)
}

// deckMarkerRE matches the deck-only directive tags. StripDeckMarkers removes them from the published
// post HTML (and feeds) so they never render as literal text — the inner content of <slide>/<noslide>
// stays, only the tags and the void <splitslide> go. (The deck builder consumes them before this.)
var deckMarkerRE = regexp.MustCompile(`(?is)</?slide>|</?noslide>|<splitslide\s*/?>`)

// StripDeckMarkers strips the deck directive tags, leaving wrapped content in place.
func StripDeckMarkers(s string) string { return deckMarkerRE.ReplaceAllString(s, "") }

// coverSlide is the leading title card: title, description, and the author (avatar or initials + date).
func coverSlide(meta deckMeta) string {
	var b strings.Builder
	b.WriteString(`<section class="slide slide-cover"><div class="cover-inner">`)
	b.WriteString(`<h1 class="cover-title">` + html.EscapeString(meta.Title) + `</h1>`)
	if meta.Description != "" {
		b.WriteString(`<p class="cover-desc">` + html.EscapeString(meta.Description) + `</p>`)
	}
	if meta.Author != "" {
		b.WriteString(`<div class="cover-by">`)
		if meta.Avatar != "" {
			b.WriteString(`<img class="cover-avatar" src="` + html.EscapeString(meta.Avatar) + `" alt="">`)
		} else {
			b.WriteString(`<span class="cover-avatar cover-initials">` + html.EscapeString(initials(meta.Author)) + `</span>`)
		}
		name := meta.Author
		if meta.Date != "" {
			name += " · " + meta.Date
		}
		b.WriteString(`<span class="cover-name">` + html.EscapeString(name) + `</span></div>`)
	}
	b.WriteString(`</div></section>`)
	return b.String()
}

// explicitSlide renders a <slide>…</slide> block verbatim as one slide: a leading heading is its
// title, everything else stays on the slide (the fit safety-net scales it if it overflows).
func explicitSlide(inner string) string {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return ""
	}
	title := ""
	if m := deckTitleRE.FindStringSubmatch(inner); m != nil {
		title, inner = m[1], strings.TrimSpace(deckTitleRE.ReplaceAllString(inner, ""))
	}
	return slideHTML(title, false, inner, "")
}

// sectionSlides splits a run of body HTML at the boundaries and paginates each chunk.
func sectionSlides(run string, bound *regexp.Regexp, meta deckMeta) []string {
	const sep = "\x00SLIDE\x00"
	var out []string
	for _, chunk := range strings.Split(bound.ReplaceAllString(run, sep+"$1"), sep) {
		out = append(out, paginateChunk(chunk, meta)...)
	}
	return out
}

// paginateChunk turns one section (heading + content) into one or more slides: the heading is the
// title, deeper headings fold into bullets, and the remaining blocks are packed to fit.
func paginateChunk(chunk string, meta deckMeta) []string {
	chunk = strings.TrimSpace(deckDividerRE.ReplaceAllString(chunk, ""))
	if chunk == "" {
		return nil
	}
	title := ""
	if m := deckTitleRE.FindStringSubmatch(chunk); m != nil {
		title = strings.TrimSpace(m[1])
		chunk = deckTitleRE.ReplaceAllString(chunk, "")
	}

	var blocks []deckBlock
	// Sub-headings below the split level fold into one leading bullet list.
	if hs := deckHeadingRE.FindAllStringSubmatch(chunk, -1); len(hs) > 0 {
		var ul strings.Builder
		ul.WriteString(`<ul class="slide-bullets">`)
		for _, h := range hs {
			ul.WriteString(`<li>` + strings.TrimSpace(h[1]) + `</li>`)
		}
		ul.WriteString(`</ul>`)
		blocks = append(blocks, deckBlock{html: ul.String(), weight: len(hs) * 2, kind: "list"})
		chunk = deckHeadingRE.ReplaceAllString(chunk, "")
	}
	blocks = append(blocks, splitBlocks(chunk)...)
	if len(blocks) == 0 {
		if title == "" {
			return nil
		}
		return []string{slideHTML(title, false, "", "")}
	}

	var out []string
	for i, page := range packBlocks(blocks, meta) {
		var body strings.Builder
		for _, bl := range page.body {
			body.WriteString(bl.html)
		}
		out = append(out, slideHTML(title, i > 0, body.String(), page.notes))
	}
	return out
}

// slideHTML wraps a title + body (+ presenter notes) into a slide section; a continuation slide marks
// the title "(cont.)". Notes are hidden in the slide view and shown only in presenter mode.
func slideHTML(title string, cont bool, body, notes string) string {
	if strings.TrimSpace(title) == "" && strings.TrimSpace(body) == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section class="slide">`)
	if title != "" {
		b.WriteString(`<h2 class="slide-title">` + title)
		if cont {
			b.WriteString(` <span class="cont">(cont.)</span>`)
		}
		b.WriteString(`</h2>`)
	}
	b.WriteString(`<div class="slide-body">` + body + `</div>`)
	if strings.TrimSpace(notes) != "" {
		b.WriteString(`<aside class="notes">` + notes + `</aside>`)
	}
	b.WriteString(`</section>`)
	return b.String()
}

// deckBlock is one top-level content block with an estimated height (in "lines") for packing.
type deckBlock struct {
	html   string
	weight int
	kind   string // code | media | mermaid | table | list | math | callout | quote | other
}

// splitBlocks parses a section fragment into its top-level blocks, each weighted by estimated height.
func splitBlocks(frag string) []deckBlock {
	frag = strings.TrimSpace(frag)
	if frag == "" {
		return nil
	}
	ctx := &xhtml.Node{Type: xhtml.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := xhtml.ParseFragment(strings.NewReader(frag), ctx)
	if err != nil {
		return []deckBlock{{html: frag, weight: textWeight(frag), kind: "other"}}
	}
	var blocks []deckBlock
	for _, n := range nodes {
		if n.Type == xhtml.TextNode {
			if strings.TrimSpace(n.Data) == "" {
				continue
			}
			blocks = append(blocks, deckBlock{html: html.EscapeString(n.Data), weight: 1, kind: "other"})
			continue
		}
		if n.Type != xhtml.ElementNode {
			continue
		}
		var buf bytes.Buffer
		if xhtml.Render(&buf, n) != nil {
			continue
		}
		h := buf.String()
		blocks = append(blocks, deckBlock{html: h, weight: blockWeight(n, h), kind: blockKind(n, h)})
	}
	return blocks
}

// blockKind classifies a node so packing can treat media/code specially.
func blockKind(n *xhtml.Node, h string) string {
	class := nodeAttr(n, "class")
	switch {
	case strings.Contains(h, "<img") || strings.Contains(h, "<video") || n.Data == "figure" || n.Data == "img" || n.Data == "video":
		return "media"
	case n.Data == "audio" || strings.Contains(h, "<audio"):
		return "media"
	case n.Data == "pre" && strings.Contains(class, "mermaid"):
		return "mermaid"
	case n.Data == "pre":
		return "code"
	case strings.Contains(class, "math-display"):
		return "math"
	case n.Data == "table" || strings.Contains(class, "table-scroll"):
		return "table"
	case n.Data == "ul" || n.Data == "ol":
		return "list"
	case strings.Contains(class, "callout"):
		return "callout"
	case n.Data == "blockquote" || strings.Contains(class, "pullquote"):
		return "quote"
	default:
		return "other"
	}
}

// blockWeight estimates a block's height in "lines" — code/tables by their real line/row counts,
// media as a fixed large chunk (it scales to fit), text by character count.
func blockWeight(n *xhtml.Node, h string) int {
	switch blockKind(n, h) {
	case "media":
		if strings.Contains(h, "<audio") {
			return 3
		}
		return 13
	case "mermaid":
		return 11
	case "code":
		// Code renders at ~.62em, so a line costs less than a body line.
		return (strings.Count(nodeText(n), "\n")+1)*7/10 + 2
	case "math":
		return 3
	case "table":
		return strings.Count(h, "<tr")*2 + 1
	case "list":
		return strings.Count(h, "<li")*2 + 1
	case "callout":
		return textWeight(nodeText(n)) + 3
	case "quote":
		return textWeight(nodeText(n)) + 2
	default:
		return textWeight(nodeText(n)) + 1
	}
}

// deckPage is one packed slide: the visible blocks plus the presenter notes (the prose) for it.
type deckPage struct {
	body  []deckBlock
	notes string
}

// packBlocks greedily fills slides to deckBudget with the non-prose blocks (media, code, tables,
// callouts, bullets…), spilling to a new slide when the next won't fit. Prose paragraphs go to the
// slide's presenter notes instead — never both. A block heavier than the budget gets its own slide;
// an oversized code block past the hard cap is truncated with a link back to the post.
func packBlocks(blocks []deckBlock, meta deckMeta) []deckPage {
	var pages []deckPage
	cur := deckPage{}
	w := 0
	flush := func() {
		if len(cur.body) > 0 || strings.TrimSpace(cur.notes) != "" {
			pages = append(pages, cur)
			cur, w = deckPage{}, 0
		}
	}
	for _, b := range blocks {
		if b.kind == "other" { // prose → presenter notes, off the slide and out of the budget
			cur.notes += b.html
			continue
		}
		if b.weight > deckBudget || (w+b.weight > deckBudget && len(cur.body) > 0) {
			flush()
		}
		if b.kind == "code" && b.weight > deckHardCap {
			b = truncateCode(b, meta.PostURL)
		}
		cur.body = append(cur.body, b)
		w += b.weight
		if b.weight > deckBudget {
			flush() // oversized block stands alone
		}
	}
	flush()
	if len(pages) == 0 {
		pages = []deckPage{{}}
	}
	return pages
}

// truncateCode keeps the first deckBudget lines of an oversized code block and appends a note linking
// to the full listing on the post.
func truncateCode(b deckBlock, postURL string) deckBlock {
	m := deckCodeRE.FindStringSubmatch(b.html)
	if m == nil {
		return b
	}
	lines := strings.Split(m[2], "\n")
	if len(lines) > deckBudget {
		lines = append(lines[:deckBudget], "…")
	}
	code := m[1] + strings.Join(lines, "\n") + m[3]
	more := `<p class="deck-more">(truncated — full listing in the post)</p>`
	if postURL != "" {
		more = `<p class="deck-more">Full listing in the <a href="` + html.EscapeString(postURL) + `">post</a> →</p>`
	}
	return deckBlock{html: code + more, weight: deckBudget + 1, kind: "code"}
}

// textWeight estimates how many lines a run of text occupies on a slide.
func textWeight(s string) int {
	n := len(strings.TrimSpace(stripTags(s)))
	if n == 0 {
		return 0
	}
	lines := (n + deckCharsPer - 1) / deckCharsPer
	if lines < 1 {
		lines = 1
	}
	return lines
}

var deckTagRE = regexp.MustCompile(`<[^>]+>`)

func stripTags(s string) string { return deckTagRE.ReplaceAllString(s, "") }

func nodeAttr(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func nodeText(n *xhtml.Node) string {
	var sb strings.Builder
	var walk func(*xhtml.Node)
	walk = func(c *xhtml.Node) {
		if c.Type == xhtml.TextNode {
			sb.WriteString(c.Data)
		}
		for ch := c.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(n)
	return sb.String()
}

func deckDoc(meta deckMeta, slides []string, body string) string {
	hHead, hScripts := deckHydration(meta.BasePath, body)
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>` + html.EscapeString(meta.Title) + `</title><style>` + deckCSS + `</style>` + hHead + `</head>` +
		`<body data-post="` + html.EscapeString(meta.PostURL) + `"><main class="deck" aria-label="Slide deck">` +
		strings.Join(slides, "\n") + `</main><script>` + deckJS + `</script>` + hScripts + `</body></html>`
}

// deckHydration emits the same KaTeX/Mermaid/highlight loaders the theme uses, from the published
// /vendor assets (no CDN), but only for what the deck actually contains.
func deckHydration(base, body string) (head, scripts string) {
	var h, s strings.Builder
	if strings.Contains(body, `class="math`) {
		h.WriteString(`<link rel="stylesheet" href="` + base + `vendor/katex/katex.min.css">`)
		s.WriteString(`<script defer src="` + base + `vendor/katex/katex.min.js" onload="document.querySelectorAll('.math').forEach(function(el){try{katex.render(el.textContent,el,{displayMode:el.classList.contains('math-display'),throwOnError:false})}catch(e){}})"></script>`)
	}
	if strings.Contains(body, "<pre") && strings.Contains(body, "<code") {
		h.WriteString(`<link rel="stylesheet" href="` + base + `vendor/highlight/github-dark.min.css">`)
		s.WriteString(`<script defer src="` + base + `vendor/highlight/highlight.min.js" onload="hljs.highlightAll()"></script>`)
	}
	if strings.Contains(body, `class="mermaid`) {
		s.WriteString(`<script defer src="` + base + `vendor/mermaid/mermaid.min.js" onload="mermaid.initialize({startOnLoad:true,theme:'dark'})"></script>`)
	}
	return h.String(), s.String()
}

// deckCSS / deckJS — inline reader (nav, presenter view, fullscreen, fit-to-viewport). Inlined so the
// downloaded file works offline (KaTeX/Mermaid/highlight still load from /vendor; full offline is a
// later step). TODO: restyle from the active theme's tokens instead of these hard-coded ones.
const deckCSS = `*{box-sizing:border-box;margin:0;padding:0}
body{font-family:Georgia,'Times New Roman',serif;background:#0e0e13;color:#ededf1}
/* No-JS default: slides stack as a readable, scrollable long-form document. */
.deck{max-width:62rem;margin:0 auto}
.slide{display:flex;flex-direction:column;justify-content:flex-start;gap:1.2rem;padding:7vh 8vw;min-height:100vh;border-bottom:1px solid #20202a}
.slide-cover{justify-content:center}
/* JS projection mode (html.js): one slide at a time, fixed to the viewport. */
html.js body{overflow:hidden}
html.js .deck{max-width:none;margin:0;position:fixed;inset:0}
html.js .slide{position:absolute;inset:0;display:none;min-height:0;border-bottom:none}
html.js .slide.active{display:flex}
.slide-title{font-size:clamp(1.7rem,5vw,3.3rem);font-weight:600;line-height:1.1;letter-spacing:-.01em;flex:none}
.slide-title .cont{font-size:.45em;font-weight:400;color:#8c8c98;letter-spacing:0}
.slide-body{font-size:clamp(1rem,2.5vw,1.7rem);line-height:1.5;transform-origin:top center;width:100%}
.slide-body>*+*{margin-top:.9rem}
.slide-body h3,.slide-body h4{font-size:1.2em;margin:.3em 0}
.slide-body ul,.slide-body ol{margin-left:1.4rem}
.slide-body li{margin:.35rem 0}
.slide-bullets{list-style:none;margin-left:0!important;display:flex;flex-direction:column;gap:.7rem}
.slide-bullets>li{margin:0;padding-left:1.6rem;position:relative}
.slide-bullets>li::before{content:"";position:absolute;left:0;top:.55em;width:.5rem;height:.5rem;background:currentColor;opacity:.55;border-radius:2px}
.slide-body pre{background:#14141a;border:1px solid #2a2a34;border-radius:10px;padding:.9rem 1.1rem;overflow:auto;font:.62em/1.5 ui-monospace,monospace}
.slide-body :not(pre)>code{font-family:ui-monospace,monospace;font-size:.88em;background:#1b1b22;padding:.05em .35em;border-radius:4px}
.slide-body table{border-collapse:collapse;font-size:.9em}
.slide-body th,.slide-body td{border-bottom:1px solid #2a2a34;padding:.4rem .7rem;text-align:left}
.slide-body img,.slide-body video{max-width:100%;max-height:64vh;border-radius:10px;display:block;margin-inline:auto}
.slide-body audio{width:100%}
.slide-body figure{margin:0}
.slide-body figcaption{font-size:.7em;color:#9a9aa6;margin-top:.4rem;text-align:center}
.slide-body blockquote{font-style:italic;border-left:3px solid #7c8cff;padding-left:1rem}
.slide-body .pullquote{border-left:3px solid #7c8cff;padding-left:1.1rem;font-style:italic}
.slide-body .pullquote figcaption{font-style:normal;text-align:left;color:#b9aaff;margin-top:.5rem}
.slide-body .callout{border:1px solid #2a2a34;border-left:3px solid #7c8cff;border-radius:8px;padding:.8rem 1.1rem;background:#15151c}
.slide-body .callout-title,.slide-body .callout-label{font-weight:600;margin-bottom:.3rem}
.slide-body .math-display{overflow-x:auto}
.deck-more{font:.72em system-ui,sans-serif;color:#9a9aa6;margin-top:.6rem}
.deck-more a{color:#b9aaff}
.notes{display:none}
.notes::before{content:"Notes";display:block;font-size:.62rem;letter-spacing:.08em;text-transform:uppercase;color:#6c6c78;margin-bottom:.3rem}
html.js .deck.presenter .slide.active{justify-content:flex-start}
html.js .deck.presenter .notes{display:block;margin-top:auto;padding-top:1rem;border-top:1px solid #2a2a34;font:1rem/1.55 system-ui,sans-serif;color:#b2b2bc}
.cover-inner{max-width:46rem}
.cover-title{font-size:clamp(2.2rem,6.5vw,4.6rem);font-weight:700;line-height:1.05;letter-spacing:-.02em}
.cover-desc{font-size:clamp(1.05rem,2.5vw,1.6rem);color:#b8b8c4;margin-top:1.2rem;line-height:1.5}
.cover-by{display:flex;align-items:center;gap:.85rem;margin-top:2.2rem}
.cover-avatar{width:3rem;height:3rem;border-radius:50%;object-fit:cover;background:#2a2a34;display:inline-flex;align-items:center;justify-content:center;font:600 1rem system-ui,sans-serif;color:#cfcfe0}
.cover-name{font:1.05rem/1.3 system-ui,sans-serif;color:#cdcdd8}
.deck-counter{position:fixed;bottom:1rem;right:1.3rem;font:600 .8rem system-ui;color:#8c8c98}
.deck-hint{position:fixed;bottom:1rem;left:1.3rem;font:.72rem system-ui;color:#55555f}
:focus-visible{outline:2px solid #7c8cff;outline-offset:3px}
@media(prefers-reduced-motion:reduce){*{transition:none!important;animation:none!important}}`

const deckJS = `(function(){
document.documentElement.className+=' js'; // switch CSS from the no-JS document to projection mode
var slides=[].slice.call(document.querySelectorAll('.slide'));if(!slides.length)return;
var deck=document.querySelector('.deck');
var post=document.body.getAttribute('data-post')||'';
var counter=document.createElement('div');counter.className='deck-counter';counter.setAttribute('aria-live','polite');document.body.appendChild(counter);
var hint=document.createElement('div');hint.className='deck-hint';hint.textContent='← → navigate · Enter play/pause · P presenter · F fullscreen · Esc exit';document.body.appendChild(hint);
var i=Math.min(slides.length-1,Math.max(0,(parseInt(location.hash.slice(1),10)||1)-1));
function fit(s){var b=s.querySelector('.slide-body');if(!b)return;b.style.transform='';var avail=s.clientHeight-b.offsetTop-24;if(avail>0&&b.scrollHeight>avail+2){b.style.transform='scale('+Math.max(0.45,avail/b.scrollHeight).toFixed(3)+')';}}
function show(n){i=Math.max(0,Math.min(slides.length-1,n));slides.forEach(function(s,k){s.classList.toggle('active',k===i);s.setAttribute('aria-hidden',k!==i);if(k===i){s.tabIndex=-1;s.focus({preventScroll:true});fit(s);}});location.hash=i+1;counter.textContent=(i+1)+' / '+slides.length;}
document.addEventListener('keydown',function(e){
if(e.key==='ArrowRight'||e.key===' '||e.key==='PageDown'){show(i+1);e.preventDefault();}
else if(e.key==='ArrowLeft'||e.key==='PageUp'){show(i-1);}
else if(e.key==='Home'){show(0);}else if(e.key==='End'){show(slides.length-1);}
else if(e.key==='Enter'){var m=slides[i].querySelector('audio,video');if(m){if(m.paused){m.play();}else{m.pause();}e.preventDefault();}}
else if(e.key==='Escape'){if(post){location.href=post;}}
else if(e.key==='p'||e.key==='P'){deck.classList.toggle('presenter');}
else if(e.key==='f'||e.key==='F'){if(!document.fullscreenElement){document.documentElement.requestFullscreen&&document.documentElement.requestFullscreen();}else{document.exitFullscreen();}}
});
window.addEventListener('hashchange',function(){var n=(parseInt(location.hash.slice(1),10)||1)-1;if(n!==i)show(n);});
window.addEventListener('resize',function(){fit(slides[i]);});
window.addEventListener('load',function(){fit(slides[i]);});
show(i);
})();`
