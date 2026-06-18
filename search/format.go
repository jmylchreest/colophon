package search

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strings"
)

// formatVersion is the emitted-index schema version; the reader refuses a higher one.
const formatVersion = 1

// excerptRunes caps the static result excerpt length; snippetTextCap caps the plain body stored
// in each fragment for query-aware snippets (a balance between snippet coverage and fragment size
// — fragments are fetched only for shown results).
const (
	excerptRunes   = 200
	snippetTextCap = 1500
)

// Writer is the minimal sink Emit writes static files to: relative path → bytes. A directory
// (DirWriter) implements it for standalone use; colophon adapts its publisher around it. Names
// are relative to wherever the caller places the index (the manifest references its siblings by
// these relative paths).
type Writer interface {
	Put(name string, data []byte) error
}

// Manifest is the mutable root of an emitted index: scoring constants, the per-doc table, and
// the shard map. Everything it points at (shards, fragments) is immutable and content-addressed,
// so only this file changes wholesale on a rebuild. See docs/design/search.md.
type Manifest struct {
	Version  int                 `json:"v"`
	Analyzer string              `json:"analyzer"`
	BM25     Params              `json:"bm25"`
	DocCount int                 `json:"docCount"`
	AvgDL    float64             `json:"avgdl"`
	Docs     map[string]DocEntry `json:"docs"`   // stable doc ID → length + fragment file
	Shards   []ShardEntry        `json:"shards"` // sorted by Lo
}

// DocEntry is a doc's BM25 length and its content-addressed fragment file.
type DocEntry struct {
	Len  int    `json:"len"`
	Frag string `json:"frag"`
}

// ShardEntry maps a first-character range [Lo,Hi] to a content-addressed postings file. A term is
// served by the shard whose range contains its first character.
type ShardEntry struct {
	Lo   string `json:"lo"`
	Hi   string `json:"hi"`
	File string `json:"file"`
}

// wirePosting is a (docID, termFreq) pair serialized compactly as a 2-element array, e.g.
// ["/posts/x/", 3]. DocIDs are the stable string IDs (not positional ints), so adding or removing
// a doc never rewrites an unrelated doc's postings — only the shards whose terms changed.
type wirePosting struct {
	ID string
	TF int
}

func (w wirePosting) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{w.ID, w.TF})
}

func (w *wirePosting) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw) != 2 {
		return fmt.Errorf("search: posting must have 2 elements, got %d", len(raw))
	}
	if err := json.Unmarshal(raw[0], &w.ID); err != nil {
		return err
	}
	return json.Unmarshal(raw[1], &w.TF)
}

type fragment struct {
	URL     string            `json:"url"`
	Title   string            `json:"title"`
	Excerpt string            `json:"excerpt"`
	Text    string            `json:"text,omitempty"` // capped plain body for query-aware snippets
	Meta    map[string]string `json:"meta,omitempty"`
}

// Build constructs an index from docs and emits the static files to dst, returning the manifest.
func Build(docs []Doc, dst Writer, opts BuildOptions) (*Manifest, error) {
	ix, err := NewIndex(docs, opts)
	if err != nil {
		return nil, err
	}
	return ix.Emit(dst)
}

// Emit writes the manifest, postings shards, and per-doc fragments to dst. Shards and fragments
// are named by a hash of their (uncompressed, canonical) content, so unchanged content yields the
// same filename and the incremental publisher skips it; the manifest is the only file that always
// changes. Output is deterministic: identical input produces byte-identical files.
func (ix *Index) Emit(dst Writer) (*Manifest, error) {
	man := &Manifest{
		Version:  formatVersion,
		Analyzer: SimpleAnalyzerID,
		BM25:     ix.params,
		DocCount: len(ix.docs),
		AvgDL:    ix.avgdl,
		Docs:     make(map[string]DocEntry, len(ix.docs)),
	}

	// Fragments — one per doc, content-addressed.
	for i := range ix.docs {
		m := ix.docs[i]
		frag := fragment{URL: m.url, Title: m.title, Excerpt: m.excerpt, Text: m.text, Meta: m.meta}
		b, err := canonicalJSON(frag)
		if err != nil {
			return nil, err
		}
		name := "fragment/" + contentHash(b) + ".json"
		if err := dst.Put(name, b); err != nil {
			return nil, err
		}
		man.Docs[m.id] = DocEntry{Len: ix.docLen[i], Frag: name}
	}

	// Shards — postings bucketed by the term's first character (a fixed, stable range), each
	// content-addressed and gzipped. Postings are already in stable-ID order (the in-memory int
	// ids are assigned by sorted string ID), so emission order is deterministic.
	buckets := map[string]shardData{}
	for term, postings := range ix.post {
		key := firstChar(term)
		wire := make([]wirePosting, len(postings))
		for j, p := range postings {
			wire[j] = wirePosting{ID: ix.docs[p.doc].id, TF: p.tf}
		}
		if buckets[key] == nil {
			buckets[key] = shardData{}
		}
		buckets[key][term] = wire
	}
	for key, data := range buckets {
		b, err := canonicalJSON(data)
		if err != nil {
			return nil, err
		}
		gz, err := gzipBytes(b)
		if err != nil {
			return nil, err
		}
		name := "index/" + contentHash(b) + ".json.gz"
		if err := dst.Put(name, gz); err != nil {
			return nil, err
		}
		man.Shards = append(man.Shards, ShardEntry{Lo: key, Hi: key, File: name})
	}
	sort.Slice(man.Shards, func(i, j int) bool { return man.Shards[i].Lo < man.Shards[j].Lo })

	b, err := canonicalJSON(man)
	if err != nil {
		return nil, err
	}
	if err := dst.Put("manifest.json", b); err != nil {
		return nil, err
	}
	return man, nil
}

type shardData map[string][]wirePosting

// Open reconstructs a queryable Index from an emitted index rooted at fsys (so "manifest.json" is
// at the root). It reads everything eagerly — the server-side/CLI query path; the browser reader
// is lazy. It errors on an unknown analyzer or a newer format version, since either means the
// query side can't faithfully reproduce build-side results.
func Open(fsys fs.FS) (*Index, error) {
	mb, err := fs.ReadFile(fsys, "manifest.json")
	if err != nil {
		return nil, err
	}
	var man Manifest
	if err := json.Unmarshal(mb, &man); err != nil {
		return nil, fmt.Errorf("search: bad manifest: %w", err)
	}
	if man.Version > formatVersion {
		return nil, fmt.Errorf("search: index format v%d is newer than supported v%d", man.Version, formatVersion)
	}
	if man.Analyzer != SimpleAnalyzerID {
		return nil, fmt.Errorf("search: index built with analyzer %q, this build only has %q", man.Analyzer, SimpleAnalyzerID)
	}

	ix := &Index{params: man.BM25, analyzer: Analyze, avgdl: man.AvgDL, post: map[string][]posting{}}

	// Intern doc IDs by sorted order so internal ints match the builder's convention.
	ids := make([]string, 0, len(man.Docs))
	for id := range man.Docs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	docNum := make(map[string]int, len(ids))
	for n, id := range ids {
		docNum[id] = n
		entry := man.Docs[id]
		frag, err := readFragment(fsys, entry.Frag)
		if err != nil {
			return nil, err
		}
		ix.docs = append(ix.docs, docMeta{
			id: id, url: frag.URL, title: frag.Title, excerpt: frag.Excerpt, text: frag.Text, meta: frag.Meta,
		})
		ix.docLen = append(ix.docLen, entry.Len)
	}

	for _, sh := range man.Shards {
		data, err := readShard(fsys, sh.File)
		if err != nil {
			return nil, err
		}
		for term, wire := range data {
			ps := make([]posting, 0, len(wire))
			for _, w := range wire {
				n, ok := docNum[w.ID]
				if !ok {
					return nil, fmt.Errorf("search: shard references unknown doc %q", w.ID)
				}
				ps = append(ps, posting{doc: n, tf: w.TF})
			}
			sort.Slice(ps, func(i, j int) bool { return ps[i].doc < ps[j].doc })
			ix.post[term] = ps
		}
	}
	return ix, nil
}

func readFragment(fsys fs.FS, name string) (fragment, error) {
	var f fragment
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return f, err
	}
	err = json.Unmarshal(b, &f)
	return f, err
}

func readShard(fsys fs.FS, name string) (shardData, error) {
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	var data shardData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return data, nil
}

// firstChar returns the first character of a term as the shard bucket key. Terms are non-empty
// (the analyzer drops empties) and start with a letter or number, so the key is filesystem- and
// URL-safe even though shard filenames are hash-only.
func firstChar(term string) string {
	for _, r := range term {
		return string(r)
	}
	return ""
}

// canonicalJSON marshals v deterministically: encoding/json sorts map keys, and we control slice
// order, so identical input yields identical bytes — the basis for content-addressing.
func canonicalJSON(v any) ([]byte, error) {
	return json.Marshal(v)
}

// contentHash is a short, stable fingerprint of content for filenames. It hashes the uncompressed
// canonical bytes, so the name is stable even if the gzip encoder changes across toolchains.
func contentHash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// gzipBytes compresses deterministically: a fixed level and the stdlib's default header (zero
// modtime, no name) mean identical input produces identical output.
func gzipBytes(b []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(b); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// capText is the plain, whitespace-collapsed body capped at maxRunes — the source the reader
// builds query-aware snippets from. Unlike makeExcerpt it adds no ellipsis (it isn't shown as-is).
func capText(body string, maxRunes int) string {
	joined := strings.Join(strings.Fields(body), " ")
	r := []rune(joined)
	if len(r) <= maxRunes {
		return joined
	}
	return string(r[:maxRunes])
}

// makeExcerpt builds a clean, whitespace-collapsed snippet of at most maxRunes runes, cut at a
// word boundary.
func makeExcerpt(body string, maxRunes int) string {
	joined := strings.Join(strings.Fields(body), " ")
	r := []rune(joined)
	if len(r) <= maxRunes {
		return joined
	}
	cut := string(r[:maxRunes])
	if i := strings.LastIndex(cut, " "); i > 0 {
		cut = cut[:i]
	}
	return cut + "…"
}
