package search

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Doc is anything indexable. The engine knows nothing about colophon pages — a caller maps its
// own content onto this. ID must be stable across builds (colophon uses the page URL): postings
// are keyed by an integer interned from the sorted IDs, so a stable ID keeps an unrelated doc's
// postings byte-identical when the set changes (see docs/design/search.md, Determinism).
type Doc struct {
	ID    string
	URL   string
	Title string
	Body  string            // already-extracted plain text
	Meta  map[string]string // shown on the result card; not indexed
}

// Params are the BM25 tuning constants. The JSON tags are the wire form the browser reader keys
// on (m.bm25.k1 / m.bm25.b).
type Params struct {
	K1 float64 `json:"k1"` // term-frequency saturation
	B  float64 `json:"b"`  // length normalization
}

// DefaultParams are the conventional BM25 defaults.
func DefaultParams() Params { return Params{K1: 1.2, B: 0.75} }

// Analyzer turns text into terms. The same Analyzer must build and query an index — see Analyze.
type Analyzer func(string) []string

// BuildOptions configure index construction. The zero value is valid (simple analyzer, default
// BM25).
type BuildOptions struct {
	Analyzer Analyzer
	BM25     Params
}

func (o BuildOptions) withDefaults() BuildOptions {
	if o.Analyzer == nil {
		o.Analyzer = Analyze
	}
	if o.BM25 == (Params{}) {
		o.BM25 = DefaultParams()
	}
	return o
}

// posting records that a term occurs in a doc tf times.
type posting struct {
	doc int
	tf  int
}

type docMeta struct {
	id      string
	url     string
	title   string
	excerpt string
	text    string // capped plain body, for query-aware snippets + highlighting in the reader
	meta    map[string]string
}

// Index is an in-memory inverted index with BM25 statistics — the shared core behind both the
// CLI query surface and the static-index emitter. Title and Body are indexed as one field in v1
// (field weighting is future work).
type Index struct {
	params   Params
	analyzer Analyzer
	docs     []docMeta // interned int id → metadata
	docLen   []int     // interned int id → token count
	avgdl    float64
	post     map[string][]posting // term → postings, sorted by doc
}

// NewIndex builds an in-memory index from docs. Integer doc ids are interned by sorted stable
// ID, so the assignment is deterministic. It errors on an empty or duplicate ID.
func NewIndex(docs []Doc, opts BuildOptions) (*Index, error) {
	opts = opts.withDefaults()

	sorted := append([]Doc(nil), docs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })

	ix := &Index{params: opts.BM25, analyzer: opts.Analyzer, post: map[string][]posting{}}
	var totalLen int
	for n, d := range sorted {
		if d.ID == "" {
			return nil, fmt.Errorf("search: document at position %d has an empty ID", n)
		}
		if n > 0 && sorted[n-1].ID == d.ID {
			return nil, fmt.Errorf("search: duplicate document ID %q", d.ID)
		}
		ix.docs = append(ix.docs, docMeta{
			id: d.ID, url: d.URL, title: d.Title,
			excerpt: makeExcerpt(d.Body, excerptRunes),
			text:    capText(d.Body, snippetTextCap),
			meta:    d.Meta,
		})

		tokens := opts.Analyzer(d.Title + " " + d.Body)
		ix.docLen = append(ix.docLen, len(tokens))
		totalLen += len(tokens)

		freq := map[string]int{}
		for _, t := range tokens {
			freq[t]++
		}
		// n increases monotonically, so each term's postings stay sorted by doc.
		for term, f := range freq {
			ix.post[term] = append(ix.post[term], posting{doc: n, tf: f})
		}
	}
	if len(ix.docs) > 0 {
		ix.avgdl = float64(totalLen) / float64(len(ix.docs))
	}
	return ix, nil
}

// Len reports the number of indexed documents.
func (ix *Index) Len() int { return len(ix.docs) }

// Result is one ranked hit. Text is the capped plain body (for query-aware snippets/highlighting);
// Excerpt is a fixed leading snippet for cheap display.
type Result struct {
	ID      string
	URL     string
	Title   string
	Excerpt string
	Text    string
	Score   float64
	Meta    map[string]string
}

// Search ranks documents against the query with BM25 and returns the top limit (all, if limit <=
// 0). Matching is by prefix: each analyzed query term matches every index term that begins with
// it (so "wiki" finds "wikilinks", and an exact term is just the length-N prefix). The union of
// matched index terms is scored once each, in sorted order, so the result is deterministic and
// matches the JS reader bit-for-bit. Ties break on doc ID.
func (ix *Index) Search(query string, limit int) []Result {
	n := float64(len(ix.docs))
	scores := map[int]float64{}

	matched := map[string]struct{}{}
	for _, qt := range ix.analyzer(query) {
		for term := range ix.post {
			if strings.HasPrefix(term, qt) {
				matched[term] = struct{}{}
			}
		}
	}
	terms := make([]string, 0, len(matched))
	for t := range matched {
		terms = append(terms, t)
	}
	sort.Strings(terms)

	for _, term := range terms {
		postings := ix.post[term]
		df := float64(len(postings))
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		for _, p := range postings {
			dl := float64(ix.docLen[p.doc])
			tf := float64(p.tf)
			denom := tf + ix.params.K1*(1-ix.params.B+ix.params.B*dl/ix.avgdl)
			scores[p.doc] += idf * (tf * (ix.params.K1 + 1)) / denom
		}
	}

	results := make([]Result, 0, len(scores))
	for doc, score := range scores {
		m := ix.docs[doc]
		results = append(results, Result{
			ID: m.id, URL: m.url, Title: m.title, Excerpt: m.excerpt, Text: m.text, Score: score, Meta: m.meta,
		})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].ID < results[j].ID
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}
