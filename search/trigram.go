package search

import "sort"

// trigramPad bounds a term so short terms and word edges still yield grams. It is outside the
// analyzer's letter/number alphabet, so it can never collide with a real character.
const trigramPad = '$'

// trigrams returns a term's de-duplicated character trigrams over the padded term ($term$), as
// runes (so multibyte characters group as one). It is the candidate key for fuzzy lookup: the
// builder inverts term→trigrams into a trigram→terms index, and the query side intersects a
// query token's trigrams against it. The JS reader mirrors this exactly (see SPEC §10).
func trigrams(term string) []string {
	r := make([]rune, 0, len(term)+2)
	r = append(r, trigramPad)
	r = append(r, []rune(term)...)
	r = append(r, trigramPad)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(r))
	for i := 0; i+3 <= len(r); i++ {
		g := string(r[i : i+3])
		if _, dup := seen[g]; !dup {
			seen[g] = struct{}{}
			out = append(out, g)
		}
	}
	sort.Strings(out)
	return out
}

// levenshtein is the edit distance (insert/delete/substitute) between two strings, over runes.
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	cur := make([]int, len(rb)+1)
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

// maxEditDist is the edit-distance budget a fuzzy match must stay within, scaled to token length
// so short tokens don't match everything. Must match the JS reader.
func maxEditDist(term string) int {
	if len([]rune(term)) <= 4 {
		return 1
	}
	return 2
}
