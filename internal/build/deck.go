package build

// SPIKE (experimental) — derive a self-contained slide deck from a post's markdown. See slide.md.
// Derived-only: slides split at the shallowest heading present (else <hr>/<newslide>); the boundary
// heading is the slide title; prose paragraphs become speaker notes; every other block (lists,
// code, tables, figures, math, callouts) renders on the slide. The whole deck is one HTML file with
// CSS + the reader JS inlined, so it works offline. Rough edges remain (see TODOs); this is to
// react to, not to ship.

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"
)

var (
	deckParaRE     = regexp.MustCompile(`(?is)<p>.*?</p>`)
	deckDividerRE  = regexp.MustCompile(`(?is)^\s*(?:<hr\s*/?>|<newslide\s*/?>(?:\s*</newslide>)?)`)
	deckNoSlideRE  = regexp.MustCompile(`(?is)<noslide>.*?</noslide>`)
	deckSlideTagRE = regexp.MustCompile(`(?is)</?slide>`)
)

// BuildDeck renders markdown to a single self-contained HTML slide deck (CSS + reader JS inlined).
func BuildDeck(md, title string) (string, error) {
	// <noslide>…</noslide> is dropped from the deck entirely; <slide>…</slide> is force-kept on the
	// slide — for the spike we just unwrap it (so its prose isn't pulled into notes). TODO: proper
	// force-include of arbitrary prose onto a slide.
	md = strings.NewReplacer("<noslide>", "<!--noslide-->", "</noslide>", "<!--/noslide-->").Replace(md)
	var buf bytes.Buffer
	if err := sharedMarkdown.Convert([]byte(preprocessCallouts(md)), &buf); err != nil {
		return "", err
	}
	body := deckNoSlideRE.ReplaceAllString(buf.String(), "")
	body = deckSlideTagRE.ReplaceAllString(body, "")

	level := 2
	if strings.Contains(strings.ToLower(body), "<h1") {
		level = 1
	}
	const sep = "\x00SLIDE\x00"
	bound := regexp.MustCompile(fmt.Sprintf(`(?is)(<h%d[\s>]|<hr\b|<newslide\b)`, level))
	var slides []string
	for _, chunk := range strings.Split(bound.ReplaceAllString(body, sep+"$1"), sep) {
		if s := renderSlide(strings.TrimSpace(chunk), level); s != "" {
			slides = append(slides, s)
		}
	}
	if len(slides) == 0 {
		slides = []string{`<section class="slide"><div class="slide-body"></div></section>`}
	}
	return deckDoc(title, slides), nil
}

func renderSlide(chunk string, level int) string {
	if chunk == "" {
		return ""
	}
	chunk = deckDividerRE.ReplaceAllString(chunk, "") // drop a leading <hr>/<newslide> divider
	titleRE := regexp.MustCompile(fmt.Sprintf(`(?is)^\s*<h%d[^>]*>(.*?)</h%d>`, level, level))
	title := ""
	if m := titleRE.FindStringSubmatch(chunk); m != nil {
		title, chunk = m[1], titleRE.ReplaceAllString(chunk, "")
	}
	var notes strings.Builder // prose paragraphs become speaker notes
	slideBody := deckParaRE.ReplaceAllStringFunc(chunk, func(p string) string {
		notes.WriteString(p)
		return ""
	})
	if strings.TrimSpace(title) == "" && strings.TrimSpace(slideBody) == "" && notes.Len() == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section class="slide">`)
	if title != "" {
		b.WriteString(`<h2 class="slide-title">` + title + `</h2>`)
	}
	b.WriteString(`<div class="slide-body">` + strings.TrimSpace(slideBody) + `</div>`)
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
body{font-family:Georgia,'Times New Roman',serif;background:#0e0e13;color:#ededf1;overflow:hidden}
.deck{position:fixed;inset:0}
.slide{position:absolute;inset:0;display:none;flex-direction:column;justify-content:center;padding:7vh 9vw;gap:1.4rem}
.slide.active{display:flex}
.slide-title{font-size:clamp(2rem,6vw,4rem);font-weight:600;line-height:1.08;letter-spacing:-.01em}
.slide-body{font-size:clamp(1.1rem,2.7vw,1.9rem);line-height:1.5}
.slide-body h3{font-size:1.3em;margin:.4em 0}
.slide-body ul,.slide-body ol{margin-left:1.4rem}
.slide-body li{margin:.45rem 0}
.slide-body pre{background:#14141a;border:1px solid #2a2a34;border-radius:10px;padding:1rem 1.2rem;overflow:auto;font:.7em ui-monospace,monospace}
.slide-body code{font-family:ui-monospace,monospace;font-size:.9em}
.slide-body table{border-collapse:collapse}
.slide-body th,.slide-body td{border-bottom:1px solid #2a2a34;padding:.45rem .8rem;text-align:left}
.slide-body img{max-width:100%;max-height:56vh;border-radius:10px}
.slide-body blockquote,.slide-body .pullquote{font-style:italic;border-left:3px solid #7c8cff;padding-left:1rem}
.notes{display:none}
.deck.presenter .slide.active{justify-content:flex-start}
.deck.presenter .notes{display:block;margin-top:auto;padding-top:1rem;border-top:1px solid #2a2a34;font:1.05rem/1.5 system-ui,sans-serif;color:#b2b2bc}
.deck-counter{position:fixed;bottom:1rem;right:1.3rem;font:600 .8rem system-ui;color:#8c8c98}
.deck-hint{position:fixed;bottom:1rem;left:1.3rem;font:.72rem system-ui;color:#55555f}
:focus-visible{outline:2px solid #7c8cff;outline-offset:3px}
@media(prefers-reduced-motion:reduce){*{transition:none!important;animation:none!important}}`

const deckJS = `(function(){
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
