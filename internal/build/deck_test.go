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
	// A has a list (a visual cue) so its prose narrates from the notes; B is pure prose, so the
	// prose is the slide (not a blank title slide with the text hidden in notes).
	md := "## A\n\nnarration for A\n\n- item one\n- item two\n\n## B\n\nB is all prose"
	out, err := BuildDeck(md, "Talk", []string{"h2"})
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(out, `<section class="slide">`); n != 2 { // A + B (cover is extra)
		t.Fatalf("want 2 content slides, got %d", n)
	}
	if !strings.Contains(out, `<section class="slide slide-cover">`) || !strings.Contains(out, `<h1 class="cover-title">Talk</h1>`) {
		t.Errorf("deck should lead with a cover slide carrying the title: %s", out)
	}
	if !strings.Contains(out, `<h2 class="slide-title">A`) {
		t.Errorf("heading should be the slide title: %s", out)
	}
	// A's prose (it has a list) narrates from the presenter notes.
	if !strings.Contains(out, `<aside class="notes prose">`) || !strings.Contains(out, "narration for A") {
		t.Errorf("a visual section's prose should be in the notes: %s", out)
	}
	// B is pure prose → it renders ON the slide, never a blank slide.
	if !strings.Contains(out, "B is all prose") {
		t.Errorf("a prose-only section should render on the slide: %s", out)
	}
	if !strings.Contains(out, "<style>") || !strings.Contains(out, "<script>") {
		t.Error("deck must inline its CSS and JS")
	}
}

func TestBuildDeckSplitTargets(t *testing.T) {
	// hr boundary; a stray h1 is NOT a boundary (it titles the first section). The first section
	// (Big + its prose) and the title-less "second" prose each become a slide.
	out, _ := BuildDeck("# Big\n\nintro\n\n---\n\nsecond", "T", []string{"hr"})
	if n := strings.Count(out, `<section class="slide">`); n != 2 {
		t.Errorf("hr split: want 2 content slides, got %d", n)
	}
	// Default split (empty list) breaks on every heading; each gets a title slide.
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

func TestBuildDeckSplitslide(t *testing.T) {
	// <splitslide> forces a break between the two lists.
	out, _ := BuildDeck("- one\n\n<splitslide>\n\n- two", "T", []string{"splitslide"})
	if n := strings.Count(out, `<section class="slide">`); n != 2 {
		t.Fatalf("splitslide: want 2 slides, got %d:\n%s", n, out)
	}
}

func TestBuildDeckExplicitSlide(t *testing.T) {
	// A section wrapped with <slide>…</slide>: the author controls the slide, so its content is the
	// slide and the section's prose narrates from the notes — never auto-added to the slide.
	md := "## Topic\n\nThis prose explains the point.\n\n<slide>\n\nThe **key** point\n\n</slide>"
	out, _ := BuildDeck(md, "T", []string{"h2"})
	if n := strings.Count(out, `<section class="slide">`); n != 1 {
		t.Fatalf("want 1 content slide, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "The <strong>key</strong> point") {
		t.Error("explicit <slide> content should be on the slide")
	}
	start := strings.Index(out, `<div class="slide-body prose">`)
	slideBody := out[start : start+strings.Index(out[start:], `</div>`)]
	if strings.Contains(slideBody, "This prose explains") {
		t.Errorf("section prose must NOT be on the slide when wrapped by <slide>: %s", slideBody)
	}
	if !strings.Contains(out, `<aside class="notes prose">`) || !strings.Contains(out, "This prose explains the point") {
		t.Errorf("section prose should go to the presenter notes: %s", out)
	}
}
