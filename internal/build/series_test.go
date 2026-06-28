package build

import (
	"testing"
	"time"

	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/markdown"
)

// seriesDoc builds a sourceDoc for a dated post with optional predecessor/series frontmatter.
func seriesDoc(slug string, day int, predecessor, series string) sourceDoc {
	return sourceDoc{doc: core.Content{
		SourcePath: slug + ".md",
		Document: markdown.Document{
			Frontmatter: markdown.Frontmatter{
				Title:       slug,
				Slug:        slug,
				Date:        time.Date(2026, 6, day, 12, 0, 0, 0, time.UTC),
				Predecessor: predecessor,
				Series:      series,
			},
			Body: "body of " + slug,
		},
	}}
}

// buildWithSeries runs buildPages then computeSeries over the non-static posts, returning the
// page slice keyed by slug for assertions.
func buildWithSeries(t *testing.T, docs []sourceDoc) map[string]*page {
	t.Helper()
	now := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	pages, _, _, err := buildPages(docs, false, now, "/", "", core.SlidesConfig{}, nil, "en", nil, nil, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	var posts []*page
	for i := range pages {
		if !pages[i].Static {
			posts = append(posts, &pages[i])
		}
	}
	computeSeries(posts, nil) // nil log: validation must warn without panicking
	out := map[string]*page{}
	for _, p := range posts {
		out[p.Slug] = p
	}
	return out
}

func TestComputeSeriesLinearChain(t *testing.T) {
	// one→two→three, newest (three) sets the title; latest-wins names the chain.
	docs := []sourceDoc{
		seriesDoc("two", 11, "one", ""),
		seriesDoc("three", 12, "two", "Building a Widget"),
		seriesDoc("one", 10, "", ""),
		seriesDoc("loner", 9, "", ""), // not in any series
	}
	posts := buildWithSeries(t, docs)

	if posts["loner"].Series != nil {
		t.Error("a post with no predecessor and no successor must not be in a series")
	}

	for _, slug := range []string{"one", "two", "three"} {
		si := posts[slug].Series
		if si == nil {
			t.Fatalf("%s should be in a series", slug)
		}
		if si.name != "Building a Widget" {
			t.Errorf("%s series name = %q, want latest-wins %q", slug, si.name, "Building a Widget")
		}
		if si.total != 3 {
			t.Errorf("%s total = %d, want 3", slug, si.total)
		}
	}

	// index is 1-based from the oldest.
	if got := posts["one"].Series.index; got != 1 {
		t.Errorf("one index = %d, want 1", got)
	}
	if got := posts["two"].Series.index; got != 2 {
		t.Errorf("two index = %d, want 2", got)
	}
	if got := posts["three"].Series.index; got != 3 {
		t.Errorf("three index = %d, want 3", got)
	}

	// prev/next are the older/newer neighbours.
	if posts["one"].Series.prev != nil {
		t.Error("root has no prev")
	}
	if posts["one"].Series.next == nil || posts["one"].Series.next.Slug != "two" {
		t.Error("one.next should be two")
	}
	if posts["three"].Series.next != nil {
		t.Error("newest has no next")
	}
	if posts["three"].Series.prev == nil || posts["three"].Series.prev.Slug != "two" {
		t.Error("three.prev should be two")
	}
}

func TestSeriesVars(t *testing.T) {
	docs := []sourceDoc{
		seriesDoc("two", 11, "one", ""),
		seriesDoc("three", 12, "two", "Building a Widget"),
		seriesDoc("one", 10, "", ""),
	}
	posts := buildWithSeries(t, docs)

	vars := seriesVars(posts["two"], "/blog/")
	if vars == nil {
		t.Fatal("series member should expose vars")
	}
	if vars["series_name"] != "Building a Widget" {
		t.Errorf("series_name = %v", vars["series_name"])
	}
	if vars["series_total"] != 3 {
		t.Errorf("series_total = %v", vars["series_total"])
	}
	if vars["series_index"] != 2 {
		t.Errorf("series_index = %v", vars["series_index"])
	}

	// series_parts is NEWEST→OLDEST, basePath-anchored, with active on the current post.
	parts := vars["series_parts"].([]map[string]any)
	if len(parts) != 3 {
		t.Fatalf("series_parts len = %d, want 3", len(parts))
	}
	wantTitles := []string{"three", "two", "one"}
	wantNums := []int{3, 2, 1}
	for i, p := range parts {
		if p["title"] != wantTitles[i] {
			t.Errorf("parts[%d].title = %v, want %s", i, p["title"], wantTitles[i])
		}
		if p["number"] != wantNums[i] {
			t.Errorf("parts[%d].number = %v, want %d", i, p["number"], wantNums[i])
		}
	}
	if parts[0]["url"] != "/blog/three/" {
		t.Errorf("parts[0].url = %v, want /blog/three/", parts[0]["url"])
	}
	// active flag is true only for "two".
	for _, p := range parts {
		active := p["active"].(bool)
		if (p["title"] == "two") != active {
			t.Errorf("active mismatch for %v: %v", p["title"], active)
		}
	}

	prev := vars["series_prev"].(map[string]any)
	if prev["url"] != "/blog/one/" {
		t.Errorf("series_prev.url = %v, want /blog/one/", prev["url"])
	}
	next := vars["series_next"].(map[string]any)
	if next["url"] != "/blog/three/" {
		t.Errorf("series_next.url = %v, want /blog/three/", next["url"])
	}

	if seriesVars(&page{}, "/") != nil {
		t.Error("a non-series page must expose no series vars")
	}
}

func TestSeriesUntitledName(t *testing.T) {
	// No member sets series: → untitled chain (name ""), but still a series.
	docs := []sourceDoc{
		seriesDoc("b", 11, "a", ""),
		seriesDoc("a", 10, "", ""),
	}
	posts := buildWithSeries(t, docs)
	if posts["a"].Series == nil || posts["a"].Series.name != "" {
		t.Errorf("untitled series name = %q, want empty", posts["a"].Series.name)
	}
	// the list flag stays truthy for an untitled series.
	if seriesListName(posts["a"]) == "" {
		t.Error("untitled series member should still report a truthy series flag")
	}
	if seriesListName(&page{}) != "" {
		t.Error("non-series page should report an empty series flag")
	}
}

func TestSeriesUnresolvedPredecessorWarnsOnly(t *testing.T) {
	docs := []sourceDoc{
		seriesDoc("x", 10, "nope", ""), // predecessor doesn't resolve
	}
	posts := buildWithSeries(t, docs) // must not panic/fail with nil log
	if posts["x"].Series != nil {
		t.Error("a post with an unresolved predecessor is not in a series")
	}
}

func TestSeriesCycleBroken(t *testing.T) {
	// a→b→a: a cycle. It must be broken (no infinite loop) and not fail the build.
	docs := []sourceDoc{
		seriesDoc("a", 10, "b", ""),
		seriesDoc("b", 11, "a", ""),
	}
	posts := buildWithSeries(t, docs)
	// After breaking one edge the two still form a chain of 2 (the surviving edge), or one
	// stands alone — either way the build completed without hanging. Assert it terminated and
	// produced consistent totals when a series formed.
	for slug, p := range posts {
		if p.Series != nil && p.Series.total != len(p.Series.parts) {
			t.Errorf("%s: total %d != parts %d", slug, p.Series.total, len(p.Series.parts))
		}
	}
}

func TestSeriesBranchDeterministic(t *testing.T) {
	// two posts (b, c) name the same predecessor a → a branch. It flattens deterministically by
	// date then slug: a (day 10) → b (day 11) → c (day 12), all in one chain.
	docs := []sourceDoc{
		seriesDoc("c", 12, "a", ""),
		seriesDoc("b", 11, "a", ""),
		seriesDoc("a", 10, "", ""),
	}
	posts := buildWithSeries(t, docs)
	if posts["a"].Series == nil {
		t.Fatal("branch root should be in a series")
	}
	got := make([]string, 0, 3)
	for _, p := range posts["a"].Series.parts {
		got = append(got, p.Slug)
	}
	want := []string{"a", "b", "c"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("branch order = %v, want %v", got, want)
		}
	}
}
