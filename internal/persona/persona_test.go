package persona

import (
	"testing"
	"time"

	"github.com/jmylchreest/colophon/internal/build"
)

func mkDoc(persona, title, body string, day int) build.CorpusDoc {
	return build.CorpusDoc{
		PersonaID: persona,
		Title:     title,
		Body:      body,
		Date:      time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC),
	}
}

func TestRankByTopicPrefersRelevant(t *testing.T) {
	docs := []build.CorpusDoc{
		mkDoc("p", "Gardening", "Tomatoes and basil grow well in raised beds with compost.", 1),
		mkDoc("p", "Kubernetes", "Pods and deployments and ingress controllers in a cluster.", 2),
		mkDoc("p", "Cooking", "A simple basil pesto with garlic, oil and pine nuts.", 3),
	}
	ex := rank(docs, "kubernetes cluster ingress", 1)
	if len(ex) != 1 {
		t.Fatalf("want 1 exemplar, got %d", len(ex))
	}
	if ex[0].Title != "Kubernetes" {
		t.Errorf("topic ranking should surface Kubernetes, got %q", ex[0].Title)
	}
}

func TestRankNoTopicIsMostRecent(t *testing.T) {
	docs := []build.CorpusDoc{
		mkDoc("p", "Old", "x", 1),
		mkDoc("p", "New", "y", 9),
		mkDoc("p", "Mid", "z", 5),
	}
	ex := rank(docs, "", 2)
	if len(ex) != 2 || ex[0].Title != "New" || ex[1].Title != "Mid" {
		t.Errorf("no-topic ranking should be most-recent-first, got %+v", titles(ex))
	}
}

func TestRankTopKClampsAndExcerptTrims(t *testing.T) {
	long := ""
	for i := 0; i < 100; i++ {
		long += "word "
	}
	ex := rank([]build.CorpusDoc{mkDoc("p", "T", "# Heading\n\n"+long, 1)}, "", 5)
	if len(ex) != 1 {
		t.Fatalf("top-k must clamp to available docs, got %d", len(ex))
	}
	if len([]rune(ex[0].Excerpt)) > 282 { // 280 + ellipsis budget
		t.Errorf("excerpt should be trimmed, got %d runes", len([]rune(ex[0].Excerpt)))
	}
	if got := ex[0].Excerpt; got == "" || got[0] == '#' {
		t.Errorf("excerpt should strip markdown noise, got %q", got)
	}
}

func titles(ex []Exemplar) []string {
	out := make([]string, len(ex))
	for i, e := range ex {
		out[i] = e.Title
	}
	return out
}
