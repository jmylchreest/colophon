package build

import (
	"strings"
	"testing"
)

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
