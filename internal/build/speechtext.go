package build

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/jmylchreest/colophon/internal/core"
)

// reSpeechBlocks matches, in document order, the rendered blocks that don't read well aloud:
// mermaid diagrams, code blocks, tables, display math, and inline math. Group order is
// significant — the mermaid <pre> is tried before the generic code <pre>.
var reSpeechBlocks = regexp.MustCompile(`(?is)(<pre[^>]*class="[^"]*mermaid[^"]*"[^>]*>.*?</pre>)|(<pre[^>]*>.*?</pre>)|(<table[^>]*>.*?</table>)|(<div[^>]*class="[^"]*math[^"]*"[^>]*>.*?</div>)|(<span[^>]*class="[^"]*math[^"]*"[^>]*>.*?</span>)`)

var reLangClass = regexp.MustCompile(`(?i)class="language-([a-z0-9+#.-]+)"`)

// reInlineCode matches inline <code> spans. Fenced code (<pre><code>) is already removed by
// the block pass, so only inline spans remain by the time this runs.
var reInlineCode = regexp.MustCompile(`(?is)<code[^>]*>(.*?)</code>`)

// reHeading matches a whole heading element (h1–h6). Headings carry no trailing punctuation, so
// once tags are flattened to spaces they run straight into the next paragraph with no pause;
// we end each heading on a sentence boundary so the voice pauses before continuing.
var reHeading = regexp.MustCompile(`(?is)<h[1-6][^>]*>(.*?)</h[1-6]>`)

// Author overrides for speech: <notts>…</notts> is shown but never spoken; <tts>…</tts> is
// always read verbatim (its inner text), overriding the block cue/drop rules.
var (
	reNoTTS = regexp.MustCompile(`(?is)<notts\b[^>]*>.*?</notts>`)
	reTTS   = regexp.MustCompile(`(?is)<tts\b[^>]*>(.*?)</tts>`)
)

// speechText reduces rendered post HTML to spoken-friendly text. Prose (paragraphs, headings,
// lists, blockquotes, callouts) is read as-is; the visual blocks are handled per the transcript
// config: "cue" replaces them with a short spoken sentence, "drop" removes them silently,
// "keep" reads their text. The first cue gains a "visit the post" hint, and a closing note is
// appended when anything was cued and wrap-up is on. Cue text is part of the audio cache key,
// so editing a cued block (e.g. code) no longer regenerates the audio — only prose changes do.
func speechText(htmlStr string, t core.SpeechTranscript, str ttsStrings, acr *acronymReplacer) string {
	s := reScriptStyle.ReplaceAllString(htmlStr, " ")

	// Author overrides first: drop <notts> regions entirely; reduce <tts> regions to their
	// plain text so the block rules below never touch them (force-read, e.g. a code block you
	// DO want spoken). Tags are stripped now but entities are decoded once at the end.
	s = reNoTTS.ReplaceAllString(s, " ")
	s = reTTS.ReplaceAllStringFunc(s, func(m string) string {
		inner := reTTS.FindStringSubmatch(m)[1]
		return " " + reTag.ReplaceAllString(inner, " ") + " "
	})

	firstCue, cued := true, false
	s = reSpeechBlocks.ReplaceAllStringFunc(s, func(m string) string {
		sub := reSpeechBlocks.FindStringSubmatch(m)
		typ, desc := "", ""
		switch {
		case sub[1] != "":
			typ, desc = "diagram", mermaidKind(sub[1])
		case sub[2] != "":
			typ, desc = "code", codeLang(sub[2])
		case sub[3] != "":
			typ = "table"
		case sub[4] != "":
			typ = "math_display"
		case sub[5] != "":
			typ = "math_inline"
		}
		switch t.Block(typ) {
		case "keep":
			return m // leave it; its text is read after tags are stripped
		case "drop":
			return " "
		default: // cue
			cued = true
			sentence := cuePhrase(typ, desc, str)
			if sentence == "" {
				return " "
			}
			if firstCue {
				firstCue = false
				sentence += " " + str.Hint
			}
			return " " + sentence + " "
		}
	})

	// Inline code (single backticks) survives the block pass; handle it per inline_code:
	// spell symbols (default), drop, or keep (read verbatim).
	switch t.Block("inline_code") {
	case "drop":
		s = reInlineCode.ReplaceAllString(s, " ")
	case "keep":
		// leave it; the tag strip below reads the text verbatim
	default: // spell
		s = reInlineCode.ReplaceAllStringFunc(s, func(m string) string {
			return " " + spellCode(reInlineCode.FindStringSubmatch(m)[1], str.Symbols) + " "
		})
	}

	// End every heading on a sentence boundary so the reading pauses between a heading and the
	// text that follows (otherwise the flattened tags leave them touching). Runs after the inline
	// passes so any inline code/markup inside a heading is already handled.
	s = reHeading.ReplaceAllStringFunc(s, func(m string) string {
		text := strings.Join(strings.Fields(reTag.ReplaceAllString(reHeading.FindStringSubmatch(m)[1], " ")), " ")
		if text == "" {
			return " "
		}
		if !endsWithSentencePunct(html.UnescapeString(text)) {
			text += "."
		}
		return " " + text + " "
	})

	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	out := strings.Join(strings.Fields(s), " ")
	if t.ExpandsAcronyms() {
		out = acr.expand(out) // SSH → "Secure Shell", spoken as words
	}
	if cued && t.WrapsUp() {
		out = strings.TrimSpace(out + " " + str.WrapUp)
	}
	return out
}

// endsWithSentencePunct reports whether text already ends with punctuation that produces a
// spoken pause, so a heading like "Why WAV?" or "The plan:" isn't given a redundant full stop.
func endsWithSentencePunct(text string) bool {
	r := []rune(strings.TrimSpace(text))
	if len(r) == 0 {
		return false
	}
	switch r[len(r)-1] {
	case '.', '!', '?', ':', ';', '…':
		return true
	}
	return false
}

// spellCode reads inline-code text aloud-friendly: HTML entities are decoded, then each
// mispronounced symbol becomes a spoken word ("/etc" → "slash etc") in the post's language;
// letters/digits pass through for the engine to voice.
func spellCode(code string, symbols map[string]string) string {
	var b strings.Builder
	for _, r := range html.UnescapeString(code) {
		if w, ok := symbols[string(r)]; ok {
			b.WriteByte(' ')
			b.WriteString(w)
			b.WriteByte(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// cuePhrase builds the spoken stand-in for a skipped block, in the post's language, weaving in
// a type/language descriptor when known.
func cuePhrase(typ, desc string, str ttsStrings) string {
	switch typ {
	case "code":
		if desc != "" {
			return fmt.Sprintf(str.CodeLang, desc)
		}
		return str.Code
	case "diagram":
		if desc != "" {
			return fmt.Sprintf(str.DiagramKind, desc)
		}
		return str.Diagram
	case "table":
		return str.Table
	case "math_display":
		return str.Formula
	}
	return ""
}

func codeLang(block string) string {
	m := reLangClass.FindStringSubmatch(block)
	if len(m) < 2 {
		return ""
	}
	switch strings.ToLower(m[1]) {
	case "go":
		return "Go"
	case "js", "javascript":
		return "JavaScript"
	case "ts", "typescript":
		return "TypeScript"
	case "py", "python":
		return "Python"
	case "rs", "rust":
		return "Rust"
	case "sh", "bash", "shell", "zsh", "console":
		return "shell"
	case "html":
		return "HTML"
	case "css":
		return "CSS"
	case "json":
		return "JSON"
	case "yaml", "yml":
		return "YAML"
	case "sql":
		return "SQL"
	case "c":
		return "C"
	case "cpp", "c++":
		return "C++"
	case "java":
		return "Java"
	case "rb", "ruby":
		return "Ruby"
	default:
		l := m[1]
		return strings.ToUpper(l[:1]) + l[1:]
	}
}

// mermaidKind reads the diagram type from the first token of a mermaid block's source.
func mermaidKind(block string) string {
	i := strings.IndexByte(block, '>')
	j := strings.LastIndex(block, "</pre>")
	if i < 0 || j <= i {
		return ""
	}
	fields := strings.Fields(html.UnescapeString(block[i+1 : j]))
	if len(fields) == 0 {
		return ""
	}
	switch strings.ToLower(strings.TrimRight(fields[0], ";")) {
	case "flowchart", "graph":
		return "flowchart"
	case "sequencediagram":
		return "sequence"
	case "classdiagram":
		return "class"
	case "statediagram", "statediagram-v2":
		return "state"
	case "erdiagram":
		return "entity-relationship"
	case "gantt":
		return "Gantt"
	case "pie":
		return "pie"
	case "journey":
		return "user-journey"
	case "mindmap":
		return "mind-map"
	case "timeline":
		return "timeline"
	default:
		return ""
	}
}
