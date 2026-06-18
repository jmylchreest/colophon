package search

import "testing"

func TestTrigrams(t *testing.T) {
	got := trigrams("go")
	// "$go$" → $go, go$ (sorted)
	want := []string{"$go", "go$"}
	if !equal(got, want) {
		t.Errorf("trigrams(go) = %v, want %v", got, want)
	}
	// de-duplicated: "aaa" → "$aa","aaa","aa$"
	if g := trigrams("aaa"); !equal(g, []string{"$aa", "aa$", "aaa"}) {
		t.Errorf("trigrams(aaa) = %v", g)
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"tigris", "tigris", 0},
		{"tigirs", "tigris", 2}, // transposition = 2 edits
		{"wikilnk", "wikilink", 1},
		{"cat", "dog", 3},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Errorf("levenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSearchFuzzyFallback(t *testing.T) {
	docs := []Doc{
		{ID: "/t/", URL: "/t/", Title: "Tigris", Body: "publishing to tigris object storage"},
		{ID: "/w/", URL: "/w/", Title: "Wiki", Body: "resolving wikilinks in obsidian"},
	}
	// Without fuzzy: a typo finds nothing.
	plain, _ := NewIndex(docs, BuildOptions{})
	if got := plain.Search("wikilnk", 0); len(got) != 0 {
		t.Errorf("non-fuzzy typo matched %v, want none", ids(got))
	}
	// With fuzzy: the typo falls back to the trigram+Levenshtein candidate.
	fz, _ := NewIndex(docs, BuildOptions{Fuzzy: true})
	if got := ids(fz.Search("wikilnk", 0)); len(got) != 1 || got[0] != "/w/" {
		t.Errorf(`fuzzy Search("wikilnk") = %v, want [/w/]`, got)
	}
	// Fuzzy-prefix: a short typo reaches the longer term through its start ("wiik" → "wikilinks").
	if got := ids(fz.Search("wiik", 0)); len(got) != 1 || got[0] != "/w/" {
		t.Errorf(`fuzzy-prefix Search("wiik") = %v, want [/w/]`, got)
	}
	if d := prefixLevenshtein("wiik", "wikilinks"); d != 1 {
		t.Errorf("prefixLevenshtein(wiik, wikilinks) = %d, want 1", d)
	}
	// An exact/prefix hit must NOT trigger the fuzzy fallback (clean queries stay clean).
	if got := ids(fz.Search("tigris", 0)); len(got) != 1 || got[0] != "/t/" {
		t.Errorf(`fuzzy Search("tigris") = %v, want [/t/]`, got)
	}
}
