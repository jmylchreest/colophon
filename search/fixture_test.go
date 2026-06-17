package search

import (
	"encoding/json"
	"testing"
)

func fixtureDocs() []Doc {
	return []Doc{
		{ID: "/go/", URL: "/go/", Title: "Go", Body: "the go programming language is great for go services"},
		{ID: "/rust/", URL: "/rust/", Title: "Rust", Body: "rust is a systems programming language with great performance"},
		{ID: "/bread/", URL: "/bread/", Title: "Bread", Body: "a recipe for fresh sourdough bread at home", Meta: map[string]string{"type": "recipe"}},
		{ID: "/search/", URL: "/search/", Title: "Search", Body: "building a static search index with bm25 ranking in go"},
		{ID: "/tigris/", URL: "/tigris/", Title: "Tigris", Body: "publishing assets to tigris object storage from go"},
	}
}

var fixtureQueries = []string{"go", "programming language", "go search", "bread recipe", "object storage", "nomatchxyz"}

// TestGenerateJSFixture (re)generates the deterministic fixture and expected results that the JS
// parity test (search.test.mjs) consumes, so the browser reader is checked against the Go engine
// on real emitted bytes. Output is deterministic, so the committed fixture stays stable across runs.
func TestGenerateJSFixture(t *testing.T) {
	ix, err := NewIndex(fixtureDocs(), BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ix.Emit(DirWriter("testdata/fixture")); err != nil {
		t.Fatal(err)
	}

	expected := map[string][]map[string]any{}
	for _, q := range fixtureQueries {
		rows := []map[string]any{}
		for _, r := range ix.Search(q, 0) {
			rows = append(rows, map[string]any{"id": r.ID, "url": r.URL, "title": r.Title, "score": r.Score})
		}
		expected[q] = rows
	}
	b, err := json.MarshalIndent(expected, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := (DirWriter("testdata/fixture")).Put("expected.json", append(b, '\n')); err != nil {
		t.Fatal(err)
	}
}
