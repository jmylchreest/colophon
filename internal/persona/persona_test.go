package persona

import (
	"strings"
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
	ex := rank(docs, "kubernetes cluster ingress", 1, defaultExcerpt, defaultBudget)
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
	ex := rank(docs, "", 2, defaultExcerpt, defaultBudget)
	if len(ex) != 2 || ex[0].Title != "New" || ex[1].Title != "Mid" {
		t.Errorf("no-topic ranking should be most-recent-first, got %+v", titles(ex))
	}
}

func TestRankClampsAndPerExemplarCap(t *testing.T) {
	long := strings.Repeat("word ", 100) // 500 chars
	ex := rank([]build.CorpusDoc{mkDoc("p", "T", "# Heading\n\n"+long, 1)}, "", 5, 100, defaultBudget)
	if len(ex) != 1 {
		t.Fatalf("top-k must clamp to available docs, got %d", len(ex))
	}
	if n := len(ex[0].Excerpt); n > 101 { // 100 + ellipsis
		t.Errorf("per-exemplar cap should trim to ~100, got %d chars", n)
	}
	if !strings.HasPrefix(ex[0].Excerpt, "# Heading") { // clip preserves markdown structure
		t.Errorf("clip should keep the markdown, got %q", ex[0].Excerpt)
	}
}

func TestRankBudgetCapsTotalAndFull(t *testing.T) {
	docs := []build.CorpusDoc{
		mkDoc("p", "A", strings.Repeat("a ", 200), 3),
		mkDoc("p", "B", strings.Repeat("b ", 200), 2),
		mkDoc("p", "C", strings.Repeat("c ", 200), 1),
	}
	ex := rank(docs, "", 10, 0, 250) // full bodies (perCap 0), 250-char total budget
	if len(ex) == 0 {
		t.Fatal("expected at least one exemplar within budget")
	}
	total := 0
	for _, e := range ex {
		total += len(e.Excerpt)
	}
	if total > 250 {
		t.Errorf("budget should cap total exemplar text, got %d", total)
	}
}

func titles(ex []Exemplar) []string {
	out := make([]string, len(ex))
	for i, e := range ex {
		out[i] = e.Title
	}
	return out
}
