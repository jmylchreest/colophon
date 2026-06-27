package build

import (
	"strings"
	"testing"

	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/markdown"
)

func TestStripDeckMarkers(t *testing.T) {
	out := StripDeckMarkers(`<p>before <noslide>keep me</noslide> after</p><splitslide><slide>one</slide>`)
	for _, bad := range []string{"<noslide>", "</noslide>", "<splitslide>", "<slide>", "</slide>"} {
		if strings.Contains(out, bad) {
			t.Errorf("marker %q not stripped: %s", bad, out)
		}
	}
	if !strings.Contains(out, "keep me") || !strings.Contains(out, "one") {
		t.Errorf("wrapped content should survive: %s", out)
	}
}

func TestResolveSlides(t *testing.T) {
	site := core.SlidesConfig{Enabled: true, Split: []string{"h2"}}
	if en, sp := resolveSlides(site, nil); !en || len(sp) != 1 || sp[0] != "h2" {
		t.Errorf("nil frontmatter should inherit site: en=%v sp=%v", en, sp)
	}
	off := false
	if en, sp := resolveSlides(site, &markdown.SlidesConfig{Enabled: &off}); en || sp[0] != "h2" {
		t.Errorf("per-post disable should win but split inherit: en=%v sp=%v", en, sp)
	}
	if en, sp := resolveSlides(site, &markdown.SlidesConfig{Split: []string{"h1", "hr"}}); !en || len(sp) != 2 {
		t.Errorf("split override should replace, enabled inherit: en=%v sp=%v", en, sp)
	}
}

func TestBuildDeckSplit(t *testing.T) {
	md := "intro prose\n\n## A\n\nbody text\n\n## B\n\n- item"
	out, err := BuildDeck(md, "Talk", []string{"h2"})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out, `<section class="slide">`); n != 3 { // intro slide + A + B
		t.Fatalf("want 3 slides, got %d", n)
	}
	if !strings.Contains(out, `<h2 class="slide-title">A</h2>`) {
		t.Errorf("heading should be the slide title: %s", out)
	}
	if !strings.Contains(out, `<aside class="notes">`) {
		t.Error("prose should move to speaker notes")
	}
	// Self-contained: CSS + reader JS inlined, so it works offline.
	if !strings.Contains(out, "<style>") || !strings.Contains(out, "<script>") {
		t.Error("deck must inline its CSS and JS")
	}
}

func TestBuildDeckSplitTargets(t *testing.T) {
	// hr as the only boundary; a stray h1 is NOT a boundary unless listed.
	out, _ := BuildDeck("# Big\n\nintro\n\n---\n\nsecond", "T", []string{"hr"})
	if n := strings.Count(out, `<section class="slide">`); n != 2 {
		t.Errorf("hr split: want 2 slides, got %d", n)
	}
	// Default split (empty list) breaks on every heading.
	out2, _ := BuildDeck("# A\n\nx\n\n## B\n\ny\n\n### C\n\nz", "T", nil)
	if n := strings.Count(out2, `<section class="slide">`); n != 3 {
		t.Errorf("default split: want 3 slides, got %d", n)
	}
}

func TestBuildDeckBullets(t *testing.T) {
	// Split on h2 only → the h3s under it fold into bullets, not their own slides.
	out, _ := BuildDeck("## Topic\n\n### First\n\n### Second", "T", []string{"h2"})
	if n := strings.Count(out, `<section class="slide">`); n != 1 {
		t.Fatalf("want 1 slide, got %d", n)
	}
	if !strings.Contains(out, `<ul class="slide-bullets"><li>First</li><li>Second</li></ul>`) {
		t.Errorf("sub-headings should fold into bullets: %s", out)
	}
}

func TestBuildDeckExplicitAndSplitslide(t *testing.T) {
	// <splitslide> forces a break; <slide>…</slide> is one verbatim slide (prose stays on it).
	out, _ := BuildDeck("intro\n\n<splitslide>\n\nsecond\n\n<slide>\n\nhand-made **slide**\n\n</slide>", "T", []string{"splitslide"})
	if n := strings.Count(out, `<section class="slide">`); n != 3 {
		t.Fatalf("want 3 slides (intro / second / explicit), got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "hand-made <strong>slide</strong>") {
		t.Error("explicit slide content missing")
	}
	if strings.Contains(out, `<aside class="notes">hand-made`) {
		t.Error("explicit-slide prose must stay ON the slide, not go to notes")
	}
}
