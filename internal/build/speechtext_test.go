package build

import (
	"strings"
	"testing"

	"github.com/jmylchreest/colophon/internal/core"
)

var enTTS = embeddedTTS.For("en")

func TestSpeechTextCuesAndKeeps(t *testing.T) {
	htmlStr := `<p>Intro paragraph.</p>` +
		`<pre><code class="language-go">func main() { x := 1 >> 2 }</code></pre>` +
		`<p>Middle with <code>inlineId</code> and <span class="math math-inline">E=mc^2</span> here.</p>` +
		`<div class="math math-display">\int_0^1 x\,dx</div>` +
		`<table><tr><td>a</td><td>b</td></tr></table>` +
		`<pre class="mermaid">flowchart LR; A--&gt;B</pre>` +
		`<div class="callout callout-note"><div class="callout-title">Heads up</div><p>read me aloud</p></div>` +
		`<p>Outro.</p>`

	out := speechText(htmlStr, core.SpeechTranscript{}, enTTS, nil)

	for _, want := range []string{"Intro paragraph.", "inlineId", "read me aloud", "Heads up", "Outro."} {
		if !strings.Contains(out, want) {
			t.Errorf("kept text %q missing from: %s", want, out)
		}
	}
	for _, want := range []string{"a Go code example", "a flowchart diagram", "a table", "a formula"} {
		if !strings.Contains(out, want) {
			t.Errorf("cue %q missing from: %s", want, out)
		}
	}
	for _, bad := range []string{"func main", "E=mc", "flowchart LR"} {
		if strings.Contains(out, bad) {
			t.Errorf("unspoken source %q leaked into: %s", bad, out)
		}
	}
	if !strings.Contains(out, "Visit the post to view it.") {
		t.Errorf("first-cue hint missing: %s", out)
	}
	if !strings.Contains(out, "visit the post to see them.") {
		t.Errorf("wrap-up note missing: %s", out)
	}
}

func TestSpeechTextHeadingPause(t *testing.T) {
	// A heading with no trailing punctuation gains a full stop so it reads as its own sentence,
	// pausing before the paragraph instead of running into it.
	out := speechText(`<h2>Pronunciation dictionaries</h2><p>The honest problem.</p>`, core.SpeechTranscript{}, enTTS, nil)
	if !strings.Contains(out, "Pronunciation dictionaries. The honest problem.") {
		t.Errorf("heading should end on a sentence boundary: %q", out)
	}
	// A heading already ending in terminal punctuation is left alone (no doubled stop).
	out2 := speechText(`<h3>Why WAV?</h3><p>Because.</p>`, core.SpeechTranscript{}, enTTS, nil)
	if strings.Contains(out2, "WAV?.") || !strings.Contains(out2, "Why WAV? Because.") {
		t.Errorf("heading with existing punctuation must not gain a full stop: %q", out2)
	}
}

func TestSpeechTextDropAndKeepOverrides(t *testing.T) {
	htmlStr := `<p>A.</p><pre><code class="language-go">code</code></pre><p>B.</p>`

	drop := speechText(htmlStr, core.SpeechTranscript{Blocks: map[string]string{"code": "drop"}}, enTTS, nil)
	if strings.Contains(drop, "code example") || strings.Contains(drop, "Visit the post") {
		t.Errorf("drop should be silent: %s", drop)
	}
	if !strings.Contains(drop, "A.") || !strings.Contains(drop, "B.") {
		t.Errorf("surrounding prose lost: %s", drop)
	}

	keep := speechText(htmlStr, core.SpeechTranscript{Blocks: map[string]string{"code": "keep"}}, enTTS, nil)
	if !strings.Contains(keep, "code") {
		t.Errorf("keep should read the code text: %s", keep)
	}
}

func TestSpeechTextNoTTSAndTTS(t *testing.T) {
	htmlStr := `<p>Before <notts>secret aside</notts> after.</p>` +
		`<tts><pre><code class="language-go">read me anyway</code></pre></tts>`
	out := speechText(htmlStr, core.SpeechTranscript{}, enTTS, nil)
	if strings.Contains(out, "secret aside") {
		t.Errorf("<notts> content must not be spoken: %s", out)
	}
	if !strings.Contains(out, "Before") || !strings.Contains(out, "after.") {
		t.Errorf("text around <notts> lost: %s", out)
	}
	if !strings.Contains(out, "read me anyway") {
		t.Errorf("<tts> must force the code text to be read: %s", out)
	}
	if strings.Contains(out, "code example") {
		t.Errorf("<tts> region must NOT be cued: %s", out)
	}
}

func TestSpeechTextInlineCode(t *testing.T) {
	htmlStr := `<p>Run <code>/etc/foo &gt; out</code> now.</p>`

	spell := speechText(htmlStr, core.SpeechTranscript{}, enTTS, nil)
	for _, want := range []string{"slash etc slash foo", "greater than", "Run", "now."} {
		if !strings.Contains(spell, want) {
			t.Errorf("spell: %q missing from: %s", want, spell)
		}
	}
	if strings.Contains(spell, "/etc/foo") {
		t.Errorf("spell: raw path should be spelled out: %s", spell)
	}

	keep := speechText(htmlStr, core.SpeechTranscript{Blocks: map[string]string{"inline_code": "keep"}}, enTTS, nil)
	if !strings.Contains(keep, "/etc/foo") {
		t.Errorf("keep: should read inline code verbatim: %s", keep)
	}

	drop := speechText(htmlStr, core.SpeechTranscript{Blocks: map[string]string{"inline_code": "drop"}}, enTTS, nil)
	if strings.Contains(drop, "etc") || strings.Contains(drop, "slash") {
		t.Errorf("drop: inline code should be silent: %s", drop)
	}
}

func TestSpeechTextWrapUpDisabled(t *testing.T) {
	no := false
	htmlStr := `<p>A.</p><table><tr><td>x</td></tr></table>`
	out := speechText(htmlStr, core.SpeechTranscript{WrapUp: &no}, enTTS, nil)
	if strings.Contains(out, "visit the post to see them.") {
		t.Errorf("wrap-up should be off: %s", out)
	}
	if !strings.Contains(out, "a table") {
		t.Errorf("table still cued: %s", out)
	}
}

func TestSpeechTextI18n(t *testing.T) {
	es := embeddedTTS.For("es")
	htmlStr := `<p>Hola.</p>` +
		`<pre><code class="language-go">x</code></pre>` +
		`<p>Ruta <code>/etc/foo</code> aquí.</p>`
	out := speechText(htmlStr, core.SpeechTranscript{}, es, nil)
	// Spanish cue + Spanish symbol ("/" → "barra"), not English.
	if !strings.Contains(out, "ejemplo de código") {
		t.Errorf("spanish code cue missing: %s", out)
	}
	if !strings.Contains(out, "barra etc barra foo") {
		t.Errorf("spanish symbol spell-out missing: %s", out)
	}
	if strings.Contains(out, "code example") || strings.Contains(out, "slash") {
		t.Errorf("english leaked into spanish output: %s", out)
	}
}

func TestTTSTableFallback(t *testing.T) {
	// Unknown language falls back to English; base-language match for regional tags.
	if got := embeddedTTS.For("xx").Code; got != enTTS.Code {
		t.Errorf("unknown lang should fall back to en, got %q", got)
	}
	if got := embeddedTTS.For("es-MX").Table; !strings.Contains(got, "tabla") {
		t.Errorf("es-MX should resolve to es, got %q", got)
	}
}
