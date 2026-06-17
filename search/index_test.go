package search

import "testing"

func sampleDocs() []Doc {
	return []Doc{
		{ID: "a", URL: "/go/", Title: "Go", Body: "the go programming language is great for go"},
		{ID: "b", URL: "/rust/", Title: "Rust", Body: "rust is a systems programming language"},
		{ID: "c", URL: "/bread/", Title: "Cooking", Body: "a recipe for fresh bread"},
	}
}

func mustIndex(t *testing.T, docs []Doc) *Index {
	t.Helper()
	ix, err := NewIndex(docs, BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return ix
}

func ids(rs []Result) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.ID
	}
	return out
}

func TestSearchRanksAndFilters(t *testing.T) {
	ix := mustIndex(t, sampleDocs())

	// "go" appears only in doc a (twice) → a is the sole hit and outranks nothing else.
	if got := ids(ix.Search("go", 0)); len(got) != 1 || got[0] != "a" {
		t.Errorf(`Search("go") = %v, want [a]`, got)
	}

	// "programming language" hits a and b, not the cooking doc.
	got := ids(ix.Search("programming language", 0))
	if len(got) != 2 {
		t.Fatalf(`Search("programming language") = %v, want 2 hits`, got)
	}
	if got[0] != "a" && got[0] != "b" {
		t.Errorf("unexpected top hit %q", got[0])
	}
	for _, id := range got {
		if id == "c" {
			t.Error("cooking doc should not match a programming query")
		}
	}

	// No match → empty.
	if got := ix.Search("xyzzy nonexistent", 0); len(got) != 0 {
		t.Errorf("Search(miss) = %v, want empty", got)
	}
}

func TestSearchLimit(t *testing.T) {
	ix := mustIndex(t, sampleDocs())
	if got := ix.Search("a programming language for bread", 1); len(got) != 1 {
		t.Errorf("limit=1 returned %d results", len(got))
	}
}

func TestSearchDeterministic(t *testing.T) {
	docs := sampleDocs()
	a := mustIndex(t, docs)
	b := mustIndex(t, docs)
	q := "programming language a"
	ra, rb := a.Search(q, 0), b.Search(q, 0)
	if len(ra) != len(rb) {
		t.Fatalf("result counts differ: %d vs %d", len(ra), len(rb))
	}
	for i := range ra {
		if ra[i].ID != rb[i].ID || ra[i].Score != rb[i].Score {
			t.Errorf("result %d differs: %+v vs %+v", i, ra[i], rb[i])
		}
	}
}

func TestResultCarriesMetadata(t *testing.T) {
	ix := mustIndex(t, []Doc{
		{ID: "x", URL: "/x/", Title: "Title X", Body: "tigris publishing", Meta: map[string]string{"type": "post"}},
	})
	got := ix.Search("tigris", 0)
	if len(got) != 1 || got[0].URL != "/x/" || got[0].Title != "Title X" || got[0].Meta["type"] != "post" {
		t.Errorf("result metadata not carried through: %+v", got)
	}
}

func TestNewIndexRejectsBadIDs(t *testing.T) {
	if _, err := NewIndex([]Doc{{ID: "dup"}, {ID: "dup"}}, BuildOptions{}); err == nil {
		t.Error("duplicate IDs should error")
	}
	if _, err := NewIndex([]Doc{{ID: ""}}, BuildOptions{}); err == nil {
		t.Error("empty ID should error")
	}
}

func TestEmptyIndexSearchIsSafe(t *testing.T) {
	ix, err := NewIndex(nil, BuildOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := ix.Search("anything", 0); len(got) != 0 {
		t.Errorf("empty index returned %v", got)
	}
}
