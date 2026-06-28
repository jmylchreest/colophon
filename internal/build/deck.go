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

	slides := make([]string, 0, 16)
	slides = append(slides, coverSlide(meta))
	slides = append(slides, sectionSlides(body, bound, meta)...)

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

// explicitSectionSlide renders a section that contains one or more <slide>…</slide> blocks: the
// author has chosen the slide, so the <slide> contents (plus any other non-prose blocks) are the
// slide, and the section's PROSE narrates from the presenter notes — it's never auto-added to the
// slide. One slide for the section (the fit safety-net scales it if it overflows).
func explicitSectionSlide(title, chunk string) []string {
	var slide, notes strings.Builder
	add := func(frag string) { // content outside a <slide>: prose → notes, other blocks → slide
		for _, b := range splitBlocks(frag) {
			if b.kind == "other" {
				notes.WriteString(b.html)
			} else {
				slide.WriteString(b.html)
			}
		}
	}
	last := 0
	for _, m := range deckSlideWrapRE.FindAllStringSubmatchIndex(chunk, -1) {
		add(chunk[last:m[0]])
		slide.WriteString(strings.TrimSpace(chunk[m[2]:m[3]])) // the <slide> inner, verbatim
		last = m[1]
	}
	add(chunk[last:])
	body := strings.TrimSpace(slide.String())
	if title == "" && body == "" {
		return nil
	}
	return []string{slideHTML(title, false, body, strings.TrimSpace(notes.String()))}
}

// sectionSlides splits a run of body HTML at the boundaries and paginates each chunk.
func sectionSlides(run string, bound *regexp.Regexp, meta deckMeta) []string {
	const sep = "\x00SLIDE\x00"
	chunks := strings.Split(bound.ReplaceAllString(run, sep+"$1"), sep)
	out := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
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

	// An explicit <slide>…</slide> in this section means the author controls the slide: its content
	// is the slide and the section's prose goes to the notes, never auto-added to the slide.
	if deckSlideWrapRE.MatchString(chunk) {
		return explicitSectionSlide(title, chunk)
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

	// If the section has any visual block (list, code, image, table, quote, callout, math, diagram),
	// the prose narrates from the notes; if it's pure prose, the prose IS the slide (don't leave it
	// blank with the text hidden in notes).
	hasVisual := false
	for _, b := range blocks {
		if b.kind != "other" {
			hasVisual = true
			break
		}
	}
	packed := packBlocks(blocks, meta, hasVisual)
	out := make([]string, 0, len(packed))
	for i, page := range packed {
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
	// .prose is the theme's content class — the active theme styles blocks (quotes, callouts, code,
	// tables, mermaid) so a deck looks like the site and theme authors can style it.
	b.WriteString(`<div class="slide-body prose">` + body + `</div>`)
	if strings.TrimSpace(notes) != "" {
		b.WriteString(`<aside class="notes prose">` + notes + `</aside>`)
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

// packBlocks greedily fills slides to deckBudget, spilling to a new slide when the next block won't
// fit. When proseToNotes is set the prose paragraphs go to the slide's presenter notes instead of the
// body (the visual blocks are the slide) — never both; otherwise prose is packed onto the slide like
// any block (so a prose-only section isn't a blank slide). An oversized code block past the hard cap
// is truncated with a link back to the post.
func packBlocks(blocks []deckBlock, meta deckMeta, proseToNotes bool) []deckPage {
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
		if proseToNotes && b.kind == "other" { // prose → presenter notes, off the slide and out of the budget
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
	// Link the active theme's stylesheet first (it styles the .prose content and supplies the
	// tokens the deck chrome reads), then the deck's own structural CSS overrides layout.
	// <base> points at the post directory so the body's page-relative asset URLs (images, video,
	// audio) resolve correctly even though the deck lives one level deeper at …/<slug>/slides/.
	base := ""
	if meta.PostURL != "" {
		base = `<base href="` + html.EscapeString(meta.PostURL) + `">`
	}
	return `<!doctype html><html lang="en" data-theme="dark"><head><meta charset="utf-8">` +
		`<script>try{var t=localStorage.getItem('press-theme');document.documentElement.dataset.theme=t||(matchMedia&&matchMedia('(prefers-color-scheme: light)').matches?'light':'dark');}catch(e){}</script>` + base +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>` + html.EscapeString(meta.Title) + `</title>` +
		`<link rel="stylesheet" href="` + meta.BasePath + `style.css">` +
		`<style>` + deckCSS + `</style>` + hHead + `</head>` +
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
		// Load the lib only — the reader renders each diagram when its slide first becomes visible
		// (Mermaid can't measure a display:none container, so startOnLoad would emit empty SVGs).
		s.WriteString(`<script defer src="` + base + `vendor/mermaid/mermaid.min.js"></script>`)
	}
	return h.String(), s.String()
}

// deckCSS / deckJS — the inline reader: STRUCTURAL slide layout + nav/presenter/fullscreen/fit, reading
// the active theme's tokens. The theme's own stylesheet (linked in deckDoc) styles the .prose content
// and may style the .slide* structure too. (KaTeX/Mermaid/highlight load from /vendor; full offline is
// a later step.)
// deckCSS is STRUCTURAL only — slide layout, projection, cover, nav chrome and the fit transform.
// Colours/typography read the active theme's tokens (var(--bg)/--text/--accent/…) with fallbacks, and
// the slide content (in .prose) is styled by the theme's stylesheet, so a deck matches the site and
// theme authors can style .slide* themselves.
const deckCSS = `*{box-sizing:border-box}
body{margin:0;transition:background-color .35s ease,color .35s ease}
/* No-JS default: slides stack as a readable, scrollable document; the theme styles the content. */
.deck{max-width:62rem;margin:0 auto}
.slide{display:flex;flex-direction:column;justify-content:flex-start;gap:1rem;padding:6vh 7vw;min-height:100vh;border-bottom:1px solid var(--border,#2a2a34)}
.slide-cover{justify-content:center}
/* JS projection mode (html.js): one slide at a time, fixed to the viewport. */
html.js body{overflow:hidden}
html.js .deck{max-width:none;margin:0;position:fixed;inset:0}
html.js .slide{position:absolute;inset:0;display:none;min-height:0;border-bottom:none}
html.js .slide.active{display:flex}
.slide-title{font-family:var(--serif,Georgia,serif);font-size:clamp(1.7rem,5vw,3.3rem);font-weight:600;line-height:1.1;letter-spacing:-.01em;flex:none;margin-bottom:.7rem}
.slide-title .cont{font-size:.45em;font-weight:400;color:var(--muted,#8c8c98);letter-spacing:0}
.slide-body{font-size:clamp(1rem,2.5vw,1.6rem);transform-origin:top center;width:100%;padding:0!important}
.slide-body>*,.notes>*{animation:none!important}
.slide-body>*:first-child{margin-top:0}
.slide-body pre{font-size:.62em!important}
.slide-body img,.slide-body video{max-height:60vh;margin:.5rem auto!important}
.slide-body pre.mermaid svg{max-height:60vh}
.slide-body audio{width:100%}
.slide-bullets{list-style:none!important;margin-left:0!important;padding-left:0!important;display:flex;flex-direction:column;gap:.7rem}
.slide-bullets>li{margin:0;padding-left:1.6rem;position:relative}
.slide-bullets>li::before{content:"";position:absolute;left:0;top:.55em;width:.5rem;height:.5rem;background:currentColor;opacity:.55;border-radius:2px}
.deck-more{font:.72em var(--sans,system-ui);color:var(--muted,#9a9aa6);margin-top:.6rem}
.deck-more a{color:var(--link,#b9aaff)}
.cover-inner{max-width:46rem}
.cover-title{font-family:var(--serif,Georgia,serif);font-size:clamp(2.2rem,6.5vw,4.6rem);font-weight:700;line-height:1.05;letter-spacing:-.02em}
.cover-desc{font-size:clamp(1.05rem,2.5vw,1.6rem);color:var(--muted,#b8b8c4);margin-top:1.2rem;line-height:1.5}
.cover-by{display:flex;align-items:center;gap:.85rem;margin-top:2.2rem}
.cover-avatar{width:3rem;height:3rem;border-radius:50%;object-fit:cover;background:var(--elevated,#2a2a34);display:inline-flex;align-items:center;justify-content:center;font:600 1rem var(--sans,system-ui);color:var(--text,#cfcfe0)}
.cover-name{font:1.05rem/1.3 var(--sans,system-ui);color:var(--muted,#cdcdd8)}
.notes{display:none;padding:0}
.notes::before{content:"Notes";display:block;font-size:.62rem;letter-spacing:.08em;text-transform:uppercase;color:var(--muted,#6c6c78);margin-bottom:.3rem}
html.js .deck.presenter .slide.active{justify-content:flex-start}
html.js .deck.presenter .notes{display:block;margin-top:auto;max-height:42vh;overflow-y:auto;padding-top:1rem;border-top:1px solid var(--border,#2a2a34);font:1.05rem/1.6 var(--sans,system-ui);color:var(--text,#d6d6e0)}
.deck-counter{position:fixed;bottom:1rem;right:1.3rem;font:600 .8rem var(--sans,system-ui);color:var(--muted,#8c8c98);z-index:10}
.deck-hint{position:fixed;bottom:1rem;left:1.3rem;font:.72rem var(--sans,system-ui);color:var(--faint,#55555f);z-index:10}
/* On-screen prev/next (tap targets for touch / no-keyboard; 46px meets WCAG 2.5.5). */
.deck-nav{position:fixed;top:50%;transform:translateY(-50%);width:46px;height:46px;border:0;border-radius:50%;background:color-mix(in srgb,var(--text,#ededf1) 12%,transparent);color:var(--text,#ededf1);font:1.7rem/1 var(--serif,Georgia,serif);cursor:pointer;display:flex;align-items:center;justify-content:center;padding-bottom:.12em;opacity:.3;transition:opacity .2s;z-index:11}
.deck-nav:hover,.deck-nav:focus-visible{opacity:.92}
.deck-nav-prev{left:.7rem}
.deck-nav-next{right:.7rem}
/* Presenter-notes + fullscreen toggles — on-screen so they work on touch / without a keyboard. */
.deck-tools{position:fixed;top:.8rem;right:.9rem;display:flex;gap:.45rem;z-index:11}
.deck-tool{width:44px;height:44px;border:0;border-radius:10px;background:color-mix(in srgb,var(--text,#ededf1) 12%,transparent);color:var(--text,#ededf1);font-size:1.3rem;line-height:1;cursor:pointer;display:flex;align-items:center;justify-content:center;opacity:.4;transition:opacity .2s}
.deck-tool:hover,.deck-tool:focus-visible{opacity:.95}
.deck-tool[aria-pressed=true]{opacity:.95;background:var(--accent,#7c8cff);color:var(--on-accent,#0b0b10)}
@media(max-width:640px){.deck-nav{opacity:.5}.deck-hint{display:none}}
/* Presenter on a phone = a teleprompter card: the cue shrinks, the notes fill the screen big. */
@media(max-width:760px){
html.js .deck.presenter .slide.active{padding:4vh 6vw}
html.js .deck.presenter .slide-title{font-size:1.3rem}
html.js .deck.presenter .slide-body{font-size:.92rem;max-height:24vh;overflow:hidden;opacity:.65}
html.js .deck.presenter .notes{flex:1 1 auto;min-height:0;max-height:none;font-size:1.6rem;line-height:1.65;margin-top:.7rem}
}
:focus-visible{outline:2px solid var(--accent,#7c8cff);outline-offset:3px}
@media(prefers-reduced-motion:reduce){*{transition:none!important;animation:none!important}}`

const deckJS = `(function(){
var d=document.documentElement;d.className+=' js'; // switch CSS from the no-JS document to projection mode
var slides=[].slice.call(document.querySelectorAll('.slide'));if(!slides.length)return;
var deck=document.querySelector('.deck');
var post=document.body.getAttribute('data-post')||'';
var counter=document.createElement('div');counter.className='deck-counter';counter.setAttribute('aria-live','polite');document.body.appendChild(counter);
var hint=document.createElement('div');hint.className='deck-hint';hint.textContent='← → navigate · Enter play/pause · P presenter · +/− autocue speed · T theme · F fullscreen · Esc exit';document.body.appendChild(hint);
var i=Math.min(slides.length-1,Math.max(0,(parseInt(location.hash.slice(1),10)||1)-1));
var mermaidReady=false;
function renderMermaid(s){if(!window.mermaid)return;if(!mermaidReady){try{mermaid.initialize({startOnLoad:false,theme:(d.dataset.theme==='light'?'default':'dark')});}catch(e){}mermaidReady=true;}var nodes=s.querySelectorAll('.mermaid:not([data-processed])');if(!nodes.length)return;try{var p=mermaid.run({nodes:nodes});if(p&&p.then){p.then(function(){fit(s);});}}catch(e){}}
function fit(s){var b=s.querySelector('.slide-body');if(!b)return;b.style.transform='';var avail=s.clientHeight-b.offsetTop-24;if(avail>0&&b.scrollHeight>avail+2){b.style.transform='scale('+Math.max(0.45,avail/b.scrollHeight).toFixed(3)+')';}}
function show(n){i=Math.max(0,Math.min(slides.length-1,n));slides.forEach(function(s,k){s.classList.toggle('active',k===i);s.setAttribute('aria-hidden',k!==i);if(k===i){s.tabIndex=-1;s.focus({preventScroll:true});renderMermaid(s);fit(s);}});location.hash=i+1;updateCounter();}
function go(n){stopAuto();show(n);} // any MANUAL navigation cancels the autocue
document.addEventListener('keydown',function(e){
if(e.key==='ArrowRight'||e.key===' '||e.key==='PageDown'){go(i+1);e.preventDefault();}
else if(e.key==='ArrowLeft'||e.key==='PageUp'){go(i-1);}
else if(e.key==='Home'){go(0);}else if(e.key==='End'){go(slides.length-1);}
else if(e.key==='Enter'){var m=slides[i].querySelector('audio,video');if(m){if(m.paused){m.play();}else{m.pause();}e.preventDefault();}}
else if(e.key==='Escape'){window.close();if(post){location.href=post;}}
else if(e.key==='p'||e.key==='P'){togglePresenter();}
else if(e.key==='f'||e.key==='F'){toggleFull();}
else if(e.key==='t'||e.key==='T'){setTheme(d.dataset.theme==='dark'?'light':'dark');}
else if(e.key==='+'||e.key==='='){setCueSpeed(cueSpeed+0.1);}
else if(e.key==='-'||e.key==='_'){setCueSpeed(cueSpeed-0.1);}
});
function togglePresenter(){var on=deck.classList.toggle('presenter');if(pBtn)pBtn.setAttribute('aria-pressed',on);fit(slides[i]);}
function toggleFull(){if(!document.fullscreenElement){d.requestFullscreen&&d.requestFullscreen();}else{document.exitFullscreen();}}
function setTheme(t){d.dataset.theme=t;try{localStorage.setItem('press-theme',t);}catch(e){}if(tBtn)tBtn.setAttribute('aria-label',t==='dark'?'Switch to light theme':'Switch to dark theme');}
// Autocue: a teleprompter — auto-scroll the slide's notes, then advance. Cancelled by manual nav.
var autoOn=false,autoRAF=0,autoStart=0,autoDur=0,cueSpeed=1;
try{var _cs=parseFloat(localStorage.getItem('deck-cue-speed'));if(_cs)cueSpeed=Math.max(0.5,Math.min(3,_cs));}catch(e){}
function fmtSpeed(){return cueSpeed.toFixed(1).replace(/\.0$/,'')+'×';}
function updateCounter(){counter.textContent=(i+1)+' / '+slides.length+(autoOn?' · '+fmtSpeed():'');}
function showSpeed(on){if(slowBtn)slowBtn.style.display=on?'':'none';if(fastBtn)fastBtn.style.display=on?'':'none';}
function setCueSpeed(s){cueSpeed=Math.max(0.5,Math.min(3,Math.round(s*10)/10));try{localStorage.setItem('deck-cue-speed',cueSpeed);}catch(e){}if(autoOn){var now=performance.now();var prog=autoDur>0?Math.min(1,(now-autoStart)/autoDur):0;autoDur=slideDur(i)/cueSpeed;autoStart=now-prog*autoDur;}updateCounter();}
function notesEl(n){return slides[n]&&slides[n].querySelector('.notes');}
function slideDur(n){var el=notesEl(n);var w=el?(el.textContent.match(/\S+/g)||[]).length:0;return Math.max(4000,w/2.3*1000+1200);} // ~140 wpm × cueSpeed + buffer
function autoTick(now){if(!autoOn)return;var el=notesEl(i);var t=autoDur>0?Math.min(1,(now-autoStart)/autoDur):1;if(el&&el.scrollHeight>el.clientHeight+4){el.scrollTop=t*(el.scrollHeight-el.clientHeight);}if(t>=1){if(i<slides.length-1){beginAuto(i+1);}else{stopAuto();}return;}autoRAF=requestAnimationFrame(autoTick);}
function beginAuto(n){if(n!=null&&n!==i)show(n);if(!deck.classList.contains('presenter'))togglePresenter();autoOn=true;if(aBtn){aBtn.setAttribute('aria-pressed','true');aBtn.textContent='⏸';}showSpeed(true);autoDur=slideDur(i)/cueSpeed;autoStart=performance.now();updateCounter();cancelAnimationFrame(autoRAF);autoRAF=requestAnimationFrame(autoTick);}
function stopAuto(){if(!autoOn)return;autoOn=false;if(aBtn){aBtn.setAttribute('aria-pressed','false');aBtn.textContent='▶';}showSpeed(false);updateCounter();cancelAnimationFrame(autoRAF);}
function tool(label,glyph,fn){var b=document.createElement('button');b.type='button';b.className='deck-tool';b.setAttribute('aria-label',label);b.textContent=glyph;b.addEventListener('click',fn);return b;}
var aBtn=tool('Autocue — auto-advance with notes','▶',function(){autoOn?stopAuto():beginAuto();});aBtn.setAttribute('aria-pressed','false');
var slowBtn=tool('Autocue slower','−',function(){setCueSpeed(cueSpeed-0.1);});slowBtn.style.display='none';
var fastBtn=tool('Autocue faster','+',function(){setCueSpeed(cueSpeed+0.1);});fastBtn.style.display='none';
var pBtn=tool('Toggle presenter notes','≡',togglePresenter);pBtn.setAttribute('aria-pressed','false');
var fBtn=tool('Toggle fullscreen','⛶',toggleFull);
var tBtn=tool(d.dataset.theme==='dark'?'Switch to light theme':'Switch to dark theme','◐',function(){setTheme(d.dataset.theme==='dark'?'light':'dark');});
var tools=document.createElement('div');tools.className='deck-tools';tools.appendChild(aBtn);tools.appendChild(slowBtn);tools.appendChild(fastBtn);tools.appendChild(pBtn);tools.appendChild(fBtn);tools.appendChild(tBtn);document.body.appendChild(tools);
var prev=document.createElement('button');prev.className='deck-nav deck-nav-prev';prev.setAttribute('aria-label','Previous slide');prev.textContent='‹';prev.addEventListener('click',function(){go(i-1);});document.body.appendChild(prev);
var next=document.createElement('button');next.className='deck-nav deck-nav-next';next.setAttribute('aria-label','Next slide');next.textContent='›';next.addEventListener('click',function(){go(i+1);});document.body.appendChild(next);
var tx=0,ty=0,tskip=false;
document.addEventListener('touchstart',function(e){var t=e.changedTouches[0];tx=t.clientX;ty=t.clientY;tskip=!!(e.target.closest&&e.target.closest('audio,video,.slide-body pre,.notes'));},{passive:true});
document.addEventListener('touchend',function(e){if(tskip)return;var t=e.changedTouches[0];var dx=t.clientX-tx,dy=t.clientY-ty;if(Math.abs(dx)>45&&Math.abs(dx)>Math.abs(dy)*1.4){go(dx<0?i+1:i-1);}},{passive:true});
window.addEventListener('hashchange',function(){var n=(parseInt(location.hash.slice(1),10)||1)-1;if(n!==i)show(n);});
window.addEventListener('resize',function(){fit(slides[i]);});
window.addEventListener('load',function(){renderMermaid(slides[i]);fit(slides[i]);});
show(i);
})();`
