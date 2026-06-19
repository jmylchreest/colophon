package build

import (
	"fmt"
	"sort"
	"strings"

	"github.com/jmylchreest/colophon/internal/clog"
)

// seriesInfo is a post's computed place in a series. It is attached to a page only when the
// post belongs to a resolved series of at least two members; templates read it via seriesVars.
type seriesInfo struct {
	name  string  // series title ("" when untitled — latest-wins across the chain)
	total int     // number of parts in the chain
	index int     // this post's 1-based position counting from the oldest (Part index of total)
	parts []*page // the whole chain ordered oldest→newest
	prev  *page   // older neighbour, or nil
	next  *page   // newer neighbour, or nil
}

// computeSeries walks the backward `predecessor` edges across every post and attaches a
// seriesInfo to each page that ends up in a chain of ≥2. It mutates each page's Series field.
// Validation is non-fatal: an unresolved predecessor, a cycle or a branch warns via log (the
// "SERIES" step) and the build continues. URLs are anchored later by seriesVars at render time.
//
// posts is the chronological (newest-first) post slice from buildPages; only posts participate
// in series (standing pages are excluded by the caller).
func computeSeries(posts []*page, log *clog.Logger) {
	if len(posts) == 0 {
		return
	}

	// resolve maps a predecessor reference (lower-cased slug or bare filename) to its page, the
	// same dual keying wikilinks use so `predecessor: hello` and `predecessor: posts/hello` both
	// resolve. On a key collision the first post wins (deterministic: posts is stably ordered).
	resolve := map[string]*page{}
	addKey := func(k string, p *page) {
		k = strings.ToLower(k)
		if k == "" {
			return
		}
		if _, ok := resolve[k]; !ok {
			resolve[k] = p
		}
	}
	for _, p := range posts {
		addKey(p.Slug, p)
		addKey(base(p.Slug), p)
	}

	// prev[p] is p's resolved predecessor page (nil edge dropped). An unresolved predecessor
	// warns and leaves p without a backward edge.
	prev := map[*page]*page{}
	for _, p := range posts {
		ref := strings.TrimSpace(p.Predecessor)
		if ref == "" {
			continue
		}
		pred := resolve[strings.ToLower(ref)]
		if pred == nil {
			log.Step("SERIES", "", "warn", fmt.Sprintf("post %q names predecessor %q which is not a known post — left out of any series", p.Slug, ref))
			continue
		}
		if pred == p {
			log.Step("SERIES", "", "warn", fmt.Sprintf("post %q names itself as predecessor — ignored", p.Slug))
			continue
		}
		prev[p] = pred
	}

	// Break cycles: a chain of predecessor edges that loops back (A→B→…→A). Walk back from each
	// post; the first edge that re-enters the current walk is severed and warned.
	for _, p := range posts {
		seen := map[*page]bool{p: true}
		cur := p
		for {
			n := prev[cur]
			if n == nil {
				break
			}
			if seen[n] {
				log.Step("SERIES", "", "warn", fmt.Sprintf("predecessor cycle detected at %q → %q — breaking the link", cur.Slug, n.Slug))
				delete(prev, cur)
				break
			}
			seen[n] = true
			cur = n
		}
	}

	// Forward edges: succ[pred] is the list of posts naming pred as predecessor. Two posts naming
	// the same predecessor is a branch (a fork of a linear series); warn once per forked parent.
	succ := map[*page][]*page{}
	for p, pred := range prev {
		succ[pred] = append(succ[pred], p)
	}
	for pred, kids := range succ {
		if len(kids) > 1 {
			names := make([]string, len(kids))
			for i, k := range kids {
				names[i] = k.Slug
			}
			sort.Strings(names)
			log.Step("SERIES", "", "warn", fmt.Sprintf("posts %s share predecessor %q (a branched series) — flattening them in date order", strings.Join(names, ", "), pred.Slug))
		}
	}
	// Order each parent's successors deterministically: by date then slug, so a branch flattens
	// into a stable linear series.
	for pred := range succ {
		sortPostsByDateSlug(succ[pred])
	}

	// Roots are posts with no (surviving) predecessor edge. Each root seeds one chain; the chain
	// is the root followed by a depth-first walk of its successors in the deterministic order
	// above, yielding an oldest→newest linearisation even across branches.
	var roots []*page
	for _, p := range posts {
		if prev[p] == nil {
			roots = append(roots, p)
		}
	}
	sortPostsByDateSlug(roots)

	for _, root := range roots {
		chain := linearise(root, succ)
		if len(chain) < 2 {
			continue // a lone post is not a series
		}
		// Latest-wins name: the title from the newest member (chain is oldest→newest) that sets
		// one; "" if none do.
		name := ""
		for _, p := range chain {
			if t := strings.TrimSpace(p.SeriesTitle); t != "" {
				name = t
			}
		}
		total := len(chain)
		for i, p := range chain {
			info := &seriesInfo{
				name:  name,
				total: total,
				index: i + 1,
				parts: chain,
			}
			if i > 0 {
				info.prev = chain[i-1]
			}
			if i < total-1 {
				info.next = chain[i+1]
			}
			p.Series = info
		}
	}
}

// linearise flattens a root and its successor tree into an oldest→newest slice via depth-first
// traversal, using the deterministic successor order already set on succ.
func linearise(root *page, succ map[*page][]*page) []*page {
	var out []*page
	var walk func(*page)
	walk = func(p *page) {
		out = append(out, p)
		for _, kid := range succ[p] {
			walk(kid)
		}
	}
	walk(root)
	return out
}

// sortPostsByDateSlug orders posts oldest-first by date, breaking ties by slug, for stable
// series output regardless of input order.
func sortPostsByDateSlug(ps []*page) {
	sort.SliceStable(ps, func(i, j int) bool {
		if ps[i].Published.Equal(ps[j].Published) {
			return ps[i].Slug < ps[j].Slug
		}
		return ps[i].Published.Before(ps[j].Published)
	})
}

// seriesVars renders a page's seriesInfo into the template variables. It returns nil when the
// page is not in a series, so the caller can leave the vars unset. basePath anchors every URL
// (matching {{ base_path }}{{ p.url }} used by the index/list).
func seriesVars(p *page, basePath string) map[string]any {
	si := p.Series
	if si == nil {
		return nil
	}
	// parts ordered NEWEST→OLDEST for the left-rail display. active matches by slug, so it holds
	// whether the caller passes the original page or a copy of it.
	parts := make([]map[string]any, 0, len(si.parts))
	for i := len(si.parts) - 1; i >= 0; i-- {
		m := si.parts[i]
		parts = append(parts, map[string]any{
			"title":  m.Title,
			"url":    basePath + m.URL,
			"number": i + 1, // 1-based from the oldest
			"active": m.Slug == p.Slug,
		})
	}
	link := func(m *page) map[string]any {
		if m == nil {
			return map[string]any{}
		}
		return map[string]any{"title": m.Title, "url": basePath + m.URL}
	}
	return map[string]any{
		"series_name":  si.name,
		"series_total": si.total,
		"series_index": si.index,
		"series_parts": parts,
		"series_prev":  link(si.prev),
		"series_next":  link(si.next),
	}
}

// seriesListName returns the series title to expose on a post-list item (the `series` flag a
// theme tests with {% if p.series %}). It is the chain name for a series member — which may be
// "" for an untitled series — so to keep the flag truthy for untitled series too, an untitled
// member reports a non-empty sentinel only when it is genuinely in a series. Empty means "not in
// a series".
func seriesListName(p *page) string {
	if p.Series == nil {
		return ""
	}
	if p.Series.name != "" {
		return p.Series.name
	}
	return "series" // untitled but still part of a series → keep the flag truthy
}
