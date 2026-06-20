package build

import (
	"bytes"
	"strings"
	"testing"
)

func renderMD(t *testing.T, md string) string {
	t.Helper()
	var buf bytes.Buffer
	if err := newMarkdown().Convert([]byte(preprocessCallouts(md)), &buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestMathRendering(t *testing.T) {
	out := renderMD(t, `Inline $E = mc^2$ and display:

$$\int_0^1 x\,dx$$`)
	if !strings.Contains(out, `<span class="math math-inline">E = mc^2</span>`) {
		t.Errorf("inline math missing: %s", out)
	}
	if !strings.Contains(out, `<div class="math math-display">\int_0^1 x\,dx</div>`) {
		t.Errorf("display math missing: %s", out)
	}
}

func TestMathLeavesCurrencyAlone(t *testing.T) {
	out := renderMD(t, `It cost $5 and $10 today.`)
	if strings.Contains(out, `class="math`) {
		t.Errorf("currency was treated as math: %s", out)
	}
}

func TestMathProtectsFromEmphasis(t *testing.T) {
	// Underscores inside math must not become <em>.
	out := renderMD(t, `$a_i + b_i$`)
	if strings.Contains(out, "<em>") || !strings.Contains(out, `a_i + b_i`) {
		t.Errorf("math content mangled by emphasis: %s", out)
	}
}

func TestMermaidFence(t *testing.T) {
	out := renderMD(t, "```mermaid\nflowchart LR\nA-->B\n```")
	if !strings.Contains(out, `<pre class="mermaid">flowchart LR`) {
		t.Errorf("mermaid not emitted as enhanceable block: %s", out)
	}
	if strings.Contains(out, "language-mermaid") {
		t.Errorf("mermaid should not be a code block: %s", out)
	}
}

func TestCodeFenceKeepsLanguage(t *testing.T) {
	out := renderMD(t, "```go\nfmt.Println(\"hi\")\n```")
	if !strings.Contains(out, `<pre><code class="language-go">`) {
		t.Errorf("code language class missing: %s", out)
	}
	if !strings.Contains(out, "&#34;hi&#34;") {
		t.Errorf("code content not escaped/preserved: %s", out)
	}
}

func TestVideoEmbedRendersPlayer(t *testing.T) {
	out := renderMD(t, `![A short demo](demo.mp4)`)
	if !strings.Contains(out, `<video class="post-video" controls preload="metadata" playsinline`) {
		t.Errorf("video player not emitted: %s", out)
	}
	if !strings.Contains(out, `<source src="demo.mp4" type="video/mp4">`) {
		t.Errorf("video source/type missing: %s", out)
	}
	if !strings.Contains(out, `aria-label="A short demo"`) {
		t.Errorf("video aria-label from alt missing: %s", out)
	}
	if strings.Contains(out, "<img") {
		t.Errorf("video embed should not render an <img>: %s", out)
	}
}

func TestAudioEmbedRendersPlayer(t *testing.T) {
	out := renderMD(t, `![](clip.mp3)`)
	if !strings.Contains(out, `<audio class="post-inline-audio" controls preload="metadata" src="clip.mp3">`) {
		t.Errorf("inline audio not emitted: %s", out)
	}
}

func TestImageStillRendersAsImage(t *testing.T) {
	out := renderMD(t, `![a cat](cat.png)`)
	if !strings.Contains(out, `<img src="cat.png"`) {
		t.Errorf("normal image regressed: %s", out)
	}
	if strings.Contains(out, "<video") || strings.Contains(out, "<audio") {
		t.Errorf("image misclassified as media: %s", out)
	}
}

func TestMediaExtIgnoresQuery(t *testing.T) {
	// A routed/CDN URL with a query string should still be recognised by extension.
	out := renderMD(t, `![](https://cdn.example.com/v/demo.webm?token=abc)`)
	if !strings.Contains(out, `type="video/webm"`) {
		t.Errorf("query-string video not recognised: %s", out)
	}
}

func TestCallout(t *testing.T) {
	out := renderMD(t, "> [!warning] Heads up\n> Body with **bold**.")
	if !strings.Contains(out, `<div class="callout callout-warning" data-callout="warning">`) {
		t.Errorf("callout wrapper missing: %s", out)
	}
	if !strings.Contains(out, `<div class="callout-title">Heads up</div>`) {
		t.Errorf("callout title missing: %s", out)
	}
	if !strings.Contains(out, "<strong>bold</strong>") {
		t.Errorf("callout body markdown not parsed: %s", out)
	}
}

func TestCalloutDefaultTitle(t *testing.T) {
	out := renderMD(t, "> [!note]\n> No explicit title.")
	if !strings.Contains(out, `<div class="callout-title">Note</div>`) {
		t.Errorf("default callout title missing: %s", out)
	}
}

func TestPlainBlockquoteUntouched(t *testing.T) {
	out := renderMD(t, "> just a quote")
	if strings.Contains(out, "callout") {
		t.Errorf("plain blockquote should not become a callout: %s", out)
	}
	if !strings.Contains(out, "<blockquote>") {
		t.Errorf("plain blockquote missing: %s", out)
	}
}
