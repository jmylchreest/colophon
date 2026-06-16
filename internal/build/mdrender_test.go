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
