package build

// SPIKE (experimental) — derive a self-contained slide deck from a post's markdown. See slide.md.
// Derived: the body is split into slides at the configured boundaries (default h1–h6/<hr>/
// <splitslide>); a boundary heading is the slide title; deeper headings become bullets; prose
// paragraphs become speaker notes; every other block (code, tables, figures, math, callouts) renders
// on the slide. Author escape hatches: <slide>…</slide> is one verbatim slide, <splitslide> forces a
// break, <noslide>…</noslide> is dropped. The whole deck is one HTML file with CSS + the reader JS
// inlined, so it works offline. Rough edges remain (see TODOs); this is to react to, not to ship.

import (
	"bytes"
	"html"
	"regexp"
	"strings"
)

var (
	deckParaRE      = regexp.MustCompile(`(?is)<p>.*?</p>`)
	deckHeadingRE   = regexp.MustCompile(`(?is)<h[1-6][^>]*>(.*?)</h[1-6]>`)
	deckDividerRE   = regexp.MustCompile(`(?is)^\s*(?:<hr\s*/?>|<splitslide\s*/?>(?:\s*</splitslide>)?)`)
	deckNoSlideRE   = regexp.MustCompile(`(?is)<noslide>.*?</noslide>`)
	deckSlideWrapRE = regexp.MustCompile(`(?is)<slide>(.*?)</slide>`)
	deckTitleRE     = regexp.MustCompile(`(?is)^\s*<h[1-6][^>]*>(.*?)</h[1-6]>`)
)

// DefaultDeckSplit is the slide-boundary list when a post sets no `slides.split`: a new slide before
// each heading (any level), each <hr>, and each explicit <splitslide>. Narrow it (e.g. [h2]) to make
// deeper headings render as bullets instead of their own slide.
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

// BuildDeck renders markdown to a single self-contained HTML slide deck (CSS + reader JS inlined),
// splitting into slides at the given boundary targets (DefaultDeckSplit when empty). <slide>…</slide>
// blocks are lifted out as verbatim slides; the runs between them are auto-split.
func BuildDeck(md, title string, split []string) (string, error) {
	var buf bytes.Buffer
	if err := sharedMarkdown.Convert([]byte(preprocessCallouts(md)), &buf); err != nil {
		return "", err
	}
	return buildDeckHTML(buf.String(), title, split), nil
}

// buildDeckHTML turns already-rendered post body HTML into the deck document. The build feeds it the
// post's own rendered HTML (so gen: images, glossary, math etc. are already resolved); the CLI feeds
// it freshly converted markdown. The body still carries the <slide>/<splitslide>/<noslide> markers.
func buildDeckHTML(body, title string, split []string) string {
	body = deckNoSlideRE.ReplaceAllString(body, "") // <noslide>…</noslide> never reaches the deck
	bound := deckBoundaryRE(split)

	// Walk the body, lifting <slide>…</slide> out as explicit slides and auto-splitting the runs
	// between them, so document order is preserved.
	var slides []string
	last := 0
	for _, m := range deckSlideWrapRE.FindAllStringSubmatchIndex(body, -1) {
		slides = append(slides, autoSlides(body[last:m[0]], bound)...)
		slides = append(slides, explicitSlide(body[m[2]:m[3]]))
		last = m[1]
	}
	slides = append(slides, autoSlides(body[last:], bound)...)

	out := slides[:0]
	for _, s := range slides {
		if s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		out = []string{`<section class="slide"><div class="slide-body"></div></section>`}
	}
	return deckDoc(title, out)
}

// deckMarkerRE matches the deck-only directive tags. StripDeckMarkers removes them from the published
// post HTML (and feeds) so they never render as literal text — the inner content of <slide>/<noslide>
// stays, only the tags and the void <splitslide> go. (The deck builder consumes them before this.)
var deckMarkerRE = regexp.MustCompile(`(?is)</?slide>|</?noslide>|<splitslide\s*/?>`)

// StripDeckMarkers strips the deck directive tags, leaving wrapped content in place.
func StripDeckMarkers(s string) string { return deckMarkerRE.ReplaceAllString(s, "") }

// autoSlides splits a run of body HTML at the boundary regex and renders each chunk as a derived slide.
func autoSlides(body string, bound *regexp.Regexp) []string {
	const sep = "\x00SLIDE\x00"
	var slides []string
	for _, chunk := range strings.Split(bound.ReplaceAllString(body, sep+"$1"), sep) {
		slides = append(slides, renderSlide(strings.TrimSpace(chunk)))
	}
	return slides
}

// explicitSlide renders a <slide>…</slide> block verbatim: a leading heading is its title, everything
// else stays ON the slide (prose included — no notes extraction).
func explicitSlide(inner string) string {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return ""
	}
	title := ""
	if m := deckTitleRE.FindStringSubmatch(inner); m != nil {
		title, inner = m[1], strings.TrimSpace(deckTitleRE.ReplaceAllString(inner, ""))
	}
	var b strings.Builder
	b.WriteString(`<section class="slide">`)
	if title != "" {
		b.WriteString(`<h2 class="slide-title">` + title + `</h2>`)
	}
	b.WriteString(`<div class="slide-body">` + inner + `</div></section>`)
	return b.String()
}

func renderSlide(chunk string) string {
	if chunk == "" {
		return ""
	}
	chunk = deckDividerRE.ReplaceAllString(chunk, "") // drop a leading <hr>/<splitslide> divider
	title := ""
	if m := deckTitleRE.FindStringSubmatch(chunk); m != nil {
		title, chunk = m[1], deckTitleRE.ReplaceAllString(chunk, "")
	}
	var notes strings.Builder // prose paragraphs become speaker notes
	slideBody := deckParaRE.ReplaceAllStringFunc(chunk, func(p string) string {
		notes.WriteString(p)
		return ""
	})
	// Any heading still in the body is below the split level → fold into a bullet list.
	var bullets strings.Builder
	if hs := deckHeadingRE.FindAllStringSubmatch(slideBody, -1); len(hs) > 0 {
		bullets.WriteString(`<ul class="slide-bullets">`)
		for _, h := range hs {
			bullets.WriteString(`<li>` + strings.TrimSpace(h[1]) + `</li>`)
		}
		bullets.WriteString(`</ul>`)
		slideBody = deckHeadingRE.ReplaceAllString(slideBody, "")
	}
	slideBody = bullets.String() + strings.TrimSpace(slideBody)
	if strings.TrimSpace(title) == "" && strings.TrimSpace(slideBody) == "" && notes.Len() == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section class="slide">`)
	if title != "" {
		b.WriteString(`<h2 class="slide-title">` + title + `</h2>`)
	}
	b.WriteString(`<div class="slide-body">` + slideBody + `</div>`)
	if n := strings.TrimSpace(notes.String()); n != "" {
		b.WriteString(`<aside class="notes">` + n + `</aside>`)
	}
	b.WriteString("</section>")
	return b.String()
}

func deckDoc(title string, slides []string) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8">` +
		`<meta name="viewport" content="width=device-width,initial-scale=1">` +
		`<title>` + html.EscapeString(title) + `</title><style>` + deckCSS + `</style></head>` +
		`<body><main class="deck" aria-label="Slide deck">` + strings.Join(slides, "\n") +
		`</main><script>` + deckJS + `</script></body></html>`
}

// deckCSS / deckJS — minimal inline reader (nav, presenter view, fullscreen). Inlined so the
// downloaded file works offline. TODO: inline images as data: URIs + pre-render KaTeX/Mermaid for
// a fully offline copy; restyle from the active theme's tokens instead of these hard-coded ones.
const deckCSS = `*{box-sizing:border-box;margin:0;padding:0}
body{font-family:Georgia,'Times New Roman',serif;background:#0e0e13;color:#ededf1}
/* No-JS default: slides stack as a readable, scrollable long-form document (notes shown). */
.deck{max-width:62rem;margin:0 auto}
.slide{display:flex;flex-direction:column;justify-content:center;gap:1.4rem;padding:7vh 9vw;min-height:100vh;border-bottom:1px solid #20202a}
/* JS projection mode (html.js): one slide at a time, fixed to the viewport, notes hidden. */
html.js body{overflow:hidden}
html.js .deck{max-width:none;margin:0;position:fixed;inset:0}
html.js .slide{position:absolute;inset:0;display:none;min-height:0;border-bottom:none}
html.js .slide.active{display:flex}
.slide-title{font-size:clamp(2rem,6vw,4rem);font-weight:600;line-height:1.08;letter-spacing:-.01em}
.slide-body{font-size:clamp(1.1rem,2.7vw,1.9rem);line-height:1.5}
.slide-body h3{font-size:1.3em;margin:.4em 0}
.slide-body ul,.slide-body ol{margin-left:1.4rem}
.slide-body li{margin:.45rem 0}
.slide-bullets{list-style:none;margin:0!important;display:flex;flex-direction:column;gap:.8rem}
.slide-bullets>li{margin:0;padding-left:1.6rem;position:relative;font-size:1.05em}
.slide-bullets>li::before{content:"";position:absolute;left:0;top:.55em;width:.55rem;height:.55rem;background:currentColor;opacity:.55;border-radius:2px}
.slide-body pre{background:#14141a;border:1px solid #2a2a34;border-radius:10px;padding:1rem 1.2rem;overflow:auto;font:.7em ui-monospace,monospace}
.slide-body code{font-family:ui-monospace,monospace;font-size:.9em}
.slide-body table{border-collapse:collapse}
.slide-body th,.slide-body td{border-bottom:1px solid #2a2a34;padding:.45rem .8rem;text-align:left}
.slide-body img{max-width:100%;max-height:56vh;border-radius:10px}
.slide-body blockquote,.slide-body .pullquote{font-style:italic;border-left:3px solid #7c8cff;padding-left:1rem}
.notes{font:1.05rem/1.6 system-ui,sans-serif;color:#b2b2bc;border-top:1px solid #2a2a34;padding-top:1rem;margin-top:1rem}
.notes::before{content:"Notes";display:block;font-size:.7rem;letter-spacing:.08em;text-transform:uppercase;color:#6c6c78;margin-bottom:.35rem}
html.js .notes{display:none}
html.js .deck.presenter .slide.active{justify-content:flex-start}
html.js .deck.presenter .notes{display:block;margin-top:auto}
.deck-counter{position:fixed;bottom:1rem;right:1.3rem;font:600 .8rem system-ui;color:#8c8c98}
.deck-hint{position:fixed;bottom:1rem;left:1.3rem;font:.72rem system-ui;color:#55555f}
:focus-visible{outline:2px solid #7c8cff;outline-offset:3px}
@media(prefers-reduced-motion:reduce){*{transition:none!important;animation:none!important}}`

const deckJS = `(function(){
document.documentElement.className+=' js'; // switch CSS from the no-JS document to projection mode
var slides=[].slice.call(document.querySelectorAll('.slide'));if(!slides.length)return;
var deck=document.querySelector('.deck');
var counter=document.createElement('div');counter.className='deck-counter';counter.setAttribute('aria-live','polite');document.body.appendChild(counter);
var hint=document.createElement('div');hint.className='deck-hint';hint.textContent='← → navigate · P presenter · F fullscreen';document.body.appendChild(hint);
var i=Math.min(slides.length-1,Math.max(0,(parseInt(location.hash.slice(1),10)||1)-1));
function show(n){i=Math.max(0,Math.min(slides.length-1,n));slides.forEach(function(s,k){s.classList.toggle('active',k===i);s.setAttribute('aria-hidden',k!==i);if(k===i){s.tabIndex=-1;s.focus({preventScroll:true})}});location.hash=i+1;counter.textContent=(i+1)+' / '+slides.length;}
document.addEventListener('keydown',function(e){
if(e.key==='ArrowRight'||e.key===' '||e.key==='PageDown'){show(i+1);e.preventDefault();}
else if(e.key==='ArrowLeft'||e.key==='PageUp'){show(i-1);}
else if(e.key==='Home'){show(0);}else if(e.key==='End'){show(slides.length-1);}
else if(e.key==='p'||e.key==='P'){deck.classList.toggle('presenter');}
else if(e.key==='f'||e.key==='F'){if(!document.fullscreenElement){document.documentElement.requestFullscreen&&document.documentElement.requestFullscreen();}else{document.exitFullscreen();}}
});
window.addEventListener('hashchange',function(){var n=(parseInt(location.hash.slice(1),10)||1)-1;if(n!==i)show(n);});
show(i);
})();`
