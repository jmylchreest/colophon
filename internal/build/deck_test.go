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
	// hr as a boundary; a stray h1 is NOT a boundary unless listed.
	out, _ := BuildDeck("# Big\n\nintro\n\n---\n\nsecond", "T", []string{"hr"})
	if n := strings.Count(out, `<section class="slide">`); n != 2 {
		t.Errorf("hr split: want 2 slides, got %d", n)
	}
	// Default split (empty list) is h2/hr/newslide.
	out2, _ := BuildDeck("a\n\n## X\n\nb", "T", nil)
	if n := strings.Count(out2, `<section class="slide">`); n != 2 {
		t.Errorf("default split: want 2 slides, got %d", n)
	}
}
