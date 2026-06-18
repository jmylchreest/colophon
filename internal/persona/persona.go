// Package persona assembles "write-as" context for a blog persona: its style guide and
// references plus the most relevant exemplars drawn from that persona's own published
// content. colophon does not generate prose — it emits this context for a calling agent.
//
// Retrieval is a pure-Go BM25 over the persona's documents, built in memory per call (no
// persisted index, no embeddings, no API key). It is the zero-config default; a semantic
// layer can replace rank() later without changing the public shape.
package persona

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jmylchreest/colophon/internal/build"
	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
)

// Exemplar is one retrieved sample of a persona's writing.
type Exemplar struct {
	Title   string    `json:"title"`
	Date    time.Time `json:"date,omitempty"`
	Path    string    `json:"path"`
	Excerpt string    `json:"excerpt"`
	Score   float64   `json:"score,omitempty"`
}

// Context is the write-as bundle for a persona: identity + style + retrieved exemplars.
type Context struct {
	Persona    core.Persona `json:"persona"`
	Guide      string       `json:"guide,omitempty"`
	References []string     `json:"references,omitempty"`
	Topic      string       `json:"topic,omitempty"`
	Exemplars  []Exemplar   `json:"exemplars"`
}

// Defaults for ContextOptions when a field is left zero.
const (
	defaultTopK    = 3
	defaultExcerpt = 1200  // per-exemplar character cap
	defaultBudget  = 10000 // total characters across all exemplars
)

// ContextOptions tunes how much voice BuildContext pulls in. Zero fields take the defaults.
type ContextOptions struct {
	Topic  string   // rank exemplars against this; empty → most-recent
	Tags   []string // only draw exemplars carrying one of these tags
	TopK   int      // max number of exemplars (≤0 → defaultTopK)
	Length int      // per-exemplar character cap (≤0 → defaultExcerpt; ignored when Full)
	Full   bool     // emit each exemplar's full body (still bounded by Budget)
	Budget int      // total character budget across all exemplars (≤0 → defaultBudget)
}

// BuildContext returns the write-as context for persona id: its style guide and references, plus
// exemplars from its own content ranked by BM25 against opts.Topic (or the most recent posts when
// the topic is empty). Each exemplar's text is bounded per-exemplar (opts.Length / opts.Full) and
// the set is bounded in total (opts.Budget), so the caller controls how much voice to pull in.
func BuildContext(cfg *config.Config, id string, opts ContextOptions) (*Context, error) {
	p := Find(cfg, id)
	if p == nil {
		return nil, fmt.Errorf("unknown persona %q (have: %s)", id, strings.Join(IDs(cfg), ", "))
	}
	topK := opts.TopK
	if topK <= 0 {
		topK = defaultTopK
	}
	budget := opts.Budget
	if budget <= 0 {
		budget = defaultBudget
	}
	perCap := opts.Length // > 0 caps each exemplar; 0 means full body
	switch {
	case opts.Full:
		perCap = 0
	case perCap <= 0:
		perCap = defaultExcerpt
	}
	corpus, err := build.Corpus(cfg)
	if err != nil {
		return nil, err
	}
	// A persona's corpus is the content published as it. On a single-persona blog, posts
	// usually carry no explicit `persona:`, so unattributed docs count toward the sole persona.
	soleDefault := len(cfg.Personas) == 1
	var mine []build.CorpusDoc
	for _, d := range corpus {
		if d.PersonaID != id && (d.PersonaID != "" || !soleDefault) {
			continue
		}
		if len(opts.Tags) > 0 && !hasAnyTag(d.Tags, opts.Tags) {
			continue // narrow the corpus to exemplars carrying one of the requested tags
		}
		mine = append(mine, d)
	}
	return &Context{
		Persona:    *p,
		Guide:      p.Style.Guide,
		References: p.Style.References,
		Topic:      opts.Topic,
		Exemplars:  rank(mine, opts.Topic, topK, perCap, budget),
	}, nil
}

// hasAnyTag reports whether doc carries any of the wanted tags (case-insensitive).
func hasAnyTag(docTags, wanted []string) bool {
	for _, w := range wanted {
		for _, t := range docTags {
			if strings.EqualFold(t, w) {
				return true
			}
		}
	}
	return false
}

// Find returns the persona with the given id, or nil.
func Find(cfg *config.Config, id string) *core.Persona {
	for i := range cfg.Personas {
		if cfg.Personas[i].ID == id {
			return &cfg.Personas[i]
		}
	}
	return nil
}

// IDs lists the configured persona ids in order.
func IDs(cfg *config.Config) []string {
	out := make([]string, len(cfg.Personas))
	for i, p := range cfg.Personas {
		out[i] = p.ID
	}
	return out
}

// rank orders the docs by relevance (BM25 against topic, else most-recent) and packs their text
// into the budget: each exemplar is clipped to perCap chars (perCap ≤ 0 → full body) and the
// running total never exceeds budget — the last one is trimmed to fit. It stops at k exemplars.
func rank(docs []build.CorpusDoc, topic string, k, perCap, budget int) []Exemplar {
	if k > len(docs) {
		k = len(docs)
	}
	if k == 0 {
		return nil
	}
	scored := make([]struct {
		d build.CorpusDoc
		s float64
	}, len(docs))
	terms := tokenize(topic)
	if len(terms) > 0 {
		scores := bm25(docs, terms)
		for i := range docs {
			scored[i].d, scored[i].s = docs[i], scores[i]
		}
		sort.SliceStable(scored, func(i, j int) bool { return scored[i].s > scored[j].s })
	} else {
		for i := range docs {
			scored[i].d, scored[i].s = docs[i], 0
		}
		sort.SliceStable(scored, func(i, j int) bool { return scored[i].d.Date.After(scored[j].d.Date) })
	}
	out := make([]Exemplar, 0, k)
	used := 0
	for i := 0; i < len(scored) && len(out) < k; i++ {
		d := scored[i].d
		body := strings.TrimSpace(d.Body)
		if body == "" {
			continue // empty doc — try the next, don't waste a slot
		}
		room := budget - used
		limit := perCap // 0 ⇒ full body; the remaining budget always caps it
		if limit <= 0 || limit > room {
			limit = room
		}
		text := clip(body, limit)
		if len(text) > room {
			break // not enough budget left for another exemplar
		}
		out = append(out, Exemplar{
			Title:   d.Title,
			Date:    d.Date,
			Path:    d.SourcePath,
			Excerpt: text,
			Score:   scored[i].s,
		})
		used += len(text)
	}
	return out
}

// bm25 scores every document against the query terms (Okapi BM25, k1=1.5, b=0.75).
func bm25(docs []build.CorpusDoc, terms []string) []float64 {
	const k1, b = 1.5, 0.75
	toks := make([][]string, len(docs))
	var totalLen int
	df := map[string]int{}
	for i, d := range docs {
		toks[i] = tokenize(d.Title + " " + d.Body)
		totalLen += len(toks[i])
		seen := map[string]bool{}
		for _, t := range toks[i] {
			if !seen[t] {
				seen[t] = true
				df[t]++
			}
		}
	}
	avgLen := 1.0
	if len(docs) > 0 {
		avgLen = float64(totalLen) / float64(len(docs))
	}
	n := float64(len(docs))
	scores := make([]float64, len(docs))
	for i := range docs {
		tf := map[string]int{}
		for _, t := range toks[i] {
			tf[t]++
		}
		dl := float64(len(toks[i]))
		var s float64
		for _, q := range terms {
			f := float64(tf[q])
			if f == 0 {
				continue
			}
			idf := math.Log(1 + (n-float64(df[q])+0.5)/(float64(df[q])+0.5))
			s += idf * (f * (k1 + 1)) / (f + k1*(1-b+b*dl/avgLen))
		}
		scores[i] = s
	}
	return scores
}

var (
	wordRE   = regexp.MustCompile(`[\p{L}\p{N}]+`)
	stopword = map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "of": true, "to": true,
		"in": true, "on": true, "for": true, "is": true, "it": true, "as": true, "at": true,
		"by": true, "be": true, "this": true, "that": true, "with": true, "from": true,
	}
)

// tokenize lowercases, splits on non-alphanumeric runs, and drops short stopwords.
func tokenize(s string) []string {
	words := wordRE.FindAllString(strings.ToLower(s), -1)
	out := words[:0]
	for _, w := range words {
		if len(w) < 2 || stopword[w] {
			continue
		}
		out = append(out, w)
	}
	return out
}

// clip trims body to at most max characters at a word/line boundary (max ≤ 0 → the whole body).
// It keeps the markdown and paragraph structure intact — that is itself part of the voice — rather
// than collapsing to a single plain-text line.
func clip(body string, max int) string {
	body = strings.TrimSpace(body)
	if max <= 0 || len(body) <= max {
		return body
	}
	const ell = "…"
	lim := max - len(ell) // reserve room so the result (incl. ellipsis) stays within max bytes
	if lim < 0 {
		lim = 0
	}
	cut := body[:lim]
	if i := strings.LastIndexAny(cut, " \n\t"); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " \n\t") + ell
}
