# colophon/search index format — specification v1

> Status: **normative, stable** · analyzer id `simple-1`, manifest `v: 1`.
> This document defines the on-disk format and query contract precisely enough to implement a
> conformant reader or builder **in any language** without reading the Go. The Go module
> `github.com/jmylchreest/colophon/search` is the reference implementation; the rationale lives in
> [../docs/design/search.md](../docs/design/search.md).

The key words **MUST**, **MUST NOT**, **SHOULD**, and **MAY** are used as in RFC 2119.

---

## 1. Scope and conformance

A **search index** is a set of static files: one manifest, zero or more postings shards, and one
fragment per document. It is queried by loading the manifest, fetching only the shards a query
needs, scoring with BM25, and fetching only the fragments shown.

Two conformance roles:

- A **conformant reader** consumes an index and, for any query, returns the ranking defined in
  §7. Readers are fully language-neutral and the common case.
- A **conformant builder** emits an index that a conformant reader ranks identically to the
  reference. Builders have one extra obligation: the **analyzer** (§6) and **BM25 statistics**
  (§4) must be correct. Builders **MAY** name files however they like — content-addressing (§8)
  is a recommended optimization, **not** a conformance requirement.

Conformance is checkable against committed **test vectors** (§9): the analyzer golden vectors and
a complete example index with expected rankings.

A reader **MUST** reject an index whose manifest `v` is greater than the version it implements,
and **MUST** reject one whose `analyzer` id it does not implement (§2, §6). Either case means it
cannot faithfully reproduce rankings.

---

## 2. Versioning and compatibility

The manifest carries two compatibility fields:

- **`v`** (integer) — the format version. This document defines `v: 1`. A reader implementing
  version *N* **MUST** process any index with `v ≤ N` it understands and **MUST** refuse `v > N`.
- **`analyzer`** (string) — the tokenizer identity (§6). A reader **MUST** refuse an index whose
  `analyzer` it does not implement, because query terms would not match indexed terms.

New optional **index types** (e.g. fuzzy, semantic — §10) are added as new manifest sections and
new files; they **MUST NOT** change the meaning of existing fields, so they do not bump `v`.

---

## 3. Index layout

An index occupies a base directory (in colophon, `_search/`). All paths in the manifest are
**relative to the directory containing the manifest**.

```
<base>/
  manifest.json                 # §4 — the entry point, plain JSON (UTF-8)
  index/<name>.json.gz          # §5 — postings shards, gzip-compressed JSON
  fragment/<name>.json          # §6.? — one per document, plain JSON
```

The directory names `index/` and `fragment/` are fixed. The manifest is conventionally
`manifest.json`, but a reader is **told** its manifest filename (so it can default to
`manifest.json`): a builder **MAY** emit several differently-named manifests into one index that
**share** the same `index/` and `fragment/` files. This lets independent builds (e.g. site
environments) coexist in one bucket — each is its own mutable root over the common, immutable,
content-addressed shards/fragments, so they neither collide nor need to prune one another. The
`<name>` portion of shard/fragment paths is opaque: a reader **MUST** use the exact `file`/`frag`
strings from the manifest and **MUST NOT** derive paths itself.

A reader fetches a path by resolving it against the manifest's location (e.g.
`<base>/` + `file`). Manifest and fragments are plain JSON. **Shards are gzip-compressed at
rest**; a reader **MUST** gunzip the fetched bytes itself and **MUST NOT** assume the transport
applied `Content-Encoding: gzip`.

---

## 4. The manifest (`manifest.json`)

UTF-8 JSON object. The mutable root of the index — it changes on every rebuild; everything it
references is immutable.

| Field | Type | Meaning |
|-------|------|---------|
| `v` | integer | Format version. `1`. |
| `analyzer` | string | Analyzer id. `"simple-1"`. |
| `bm25` | object | `{ "k1": number, "b": number }` — BM25 constants (§7). |
| `docCount` | integer | `N`, the number of documents. Equals `len(docs)`. |
| `avgdl` | number | Mean document length in tokens (§7). |
| `docs` | object | Map of **document ID → `{ "len": integer, "frag": string }`**. `len` is the document's token count; `frag` is its fragment path. |
| `shards` | array | List of shard descriptors `{ "lo": string, "hi": string, "file": string }`, sorted ascending by `lo` (§5). |

Document IDs are arbitrary opaque strings chosen by the builder (colophon uses the page URL).
They are the keys used in postings (§5). Numbers (`avgdl`, `bm25.*`) are IEEE-754 doubles; a
reader parses them as such.

Example (from the test vector, formatted for readability):

```json
{
  "v": 1,
  "analyzer": "simple-1",
  "bm25": { "k1": 1.2, "b": 0.75 },
  "docCount": 5,
  "avgdl": 9.8,
  "docs": {
    "/go/":    { "len": 10, "frag": "fragment/7fe93de78720d7d4.json" },
    "/bread/": { "len": 9,  "frag": "fragment/54c9422dc65c624f.json" }
  },
  "shards": [
    { "lo": "a", "hi": "a", "file": "index/bcb75f7dfc9fc5c7.json.gz" },
    { "lo": "g", "hi": "g", "file": "index/0e07fe63e77b6c2e.json.gz" }
  ]
}
```

---

## 5. Postings shards (`index/*.json.gz`)

Each shard is a **gzip member** wrapping a UTF-8 JSON object that maps **term → postings list**:

```json
{ "go": [["/go/", 2]], "great": [["/go/", 1], ["/rust/", 1]] }
```

A **posting** is a two-element array `[docID, termFrequency]`: the document ID (a key of
`manifest.docs`) and the number of times the term occurs in that document (integer ≥ 1). A term
appears in exactly one shard. Within a term's list, postings are ordered ascending by `docID`
(a builder requirement that lets readers merge without sorting; a reader **MUST NOT** rely on it
for correctness and **MAY** re-sort).

**Sharding rule.** A shard covers all terms whose **first character** (first Unicode scalar
value) is within the inclusive range `[lo, hi]` under code-point ordering (§9.3). To find the
shard for a term, a reader takes the term's first character `c` and selects the shard with
`lo ≤ c ≤ hi`. In `v1` each shard covers a single character, so `lo == hi`; readers **MUST**
still implement range containment so future multi-character ranges work unchanged. A term whose
first character matches no shard has no postings (no documents contain it).

`docCount` (`N`) and the per-term **document frequency** `df = len(postings[term])` are the only
corpus statistics needed for scoring beyond `avgdl` and the per-document `len`.

---

## 6. Fragments (`fragment/*.json`)

One plain-JSON file per document, fetched only for results actually displayed. It carries the
human-facing result card and nothing the scorer needs:

| Field | Type | Meaning |
|-------|------|---------|
| `url` | string | The link to the document. |
| `title` | string | Display title. |
| `excerpt` | string | A short prebuilt snippet (builder-defined; e.g. the first ~200 characters). |
| `text` | string | Optional. A capped plain-text body the reader uses to build a *query-aware* highlighted snippet and an occurrence count. Omitted when empty; readers degrade to `excerpt`. |
| `meta` | object | Optional `string → string` map (e.g. `{"type":"post"}`). Omitted when empty. |

### The analyzer (`simple-1`)

Tokenization is the one contract both **builder and reader MUST share** — query tokens are matched
against indexed terms by the prefix rule (§7), so both sides **MUST** produce identical tokens.
`simple-1` is:

1. Lowercase the input (Unicode-aware case folding — Go `strings.ToLower`, JS `String.toLowerCase`).
2. Split on every maximal run of characters that are **not** a Unicode letter (category L) or
   number (category N). Equivalent to splitting on the regex `[^\p{L}\p{N}]+`.
3. Discard empty tokens (leading/trailing/repeated separators produce none).

There is **no** NFC normalization, **no** stop-word removal, and **no** stemming in `simple-1`.
Consequences a conformant implementation must accept: a *decomposed* accented character (base
letter + combining mark) tokenizes differently from a *precomposed* one, because combining marks
are category M, not L/N. Content is assumed precomposed (NFC). Adding normalization or stemming
is a **new analyzer id** (e.g. `simple-2`), never a silent change.

Conformance is pinned by the golden vectors in [`testdata/analyzer.json`](testdata/analyzer.json)
(`{ "in": string, "out": [string,…] }` cases). A conformant analyzer reproduces every `out`.

---

## 7. Querying (normative)

Matching is by **prefix**: a query token matches every index term that begins with it, so `wiki`
matches `wiki`, `wikilink`, and `wikilinks` (an exact term is just the full-length prefix). Given a
query string and a limit, a conformant reader produces results as follows.

1. **Tokenize** the query with the analyzer (§6).
2. **Expand to matched index terms.** For each query token `q`, every index term that has `q` as a
   prefix is a match. Because all of `q`'s prefix-expansions begin with the same first character,
   they live in the **same shard** (§5): a reader fetches, per query token, the single shard
   covering that token's first character, then selects the terms in it that start with `q`. Form
   the **set** of matched index terms (the union over query tokens — so a term matched by two
   tokens is still counted once), and process it in **ascending code-point order** (§9.3) so
   accumulation is deterministic.
3. **Initialize** `score[doc] = 0` for all documents (sparsely).
4. For each matched index term `t` (sorted), with `postings = shard[t]`: let `df = len(postings)`
   and compute the inverse document frequency
   **`idf = ln( 1 + (N − df + 0.5) / (df + 0.5) )`** with `N = docCount`. Then for each
   `[doc, tf]` in `postings`, with `dl = docs[doc].len`, `b = bm25.b`, `k1 = bm25.k1`,
   `avgdl = manifest.avgdl`, add to `score[doc]`:

   ```
   idf · ( tf · (k1 + 1) ) / ( tf + k1 · (1 − b + b · dl / avgdl) )
   ```

5. **Rank** documents by `score` descending. Ties **MUST** break by document ID ascending
   (code-point order, §9.3) so output is deterministic.
6. Take the top `limit` (all, if `limit ≤ 0`).
7. For each result, fetch its fragment (§6) and emit `{ id, url, title, excerpt, meta, score }`.

All arithmetic is IEEE-754 double precision. Accumulation order is term-by-term, and within a
term by the shard's posting order; following this order reproduces the reference scores
bit-for-bit, but implementations needing only correct *ranking* may accumulate in any order
(differences are below ranking significance).

A reader **MUST** fetch only the shards for the query's terms and only the fragments for the
results it returns — never the whole index.

---

## 8. Determinism and content-addressing (builder guidance)

These rules make rebuilds cheap (an incremental publisher uploads only changed files) and let a
CDN cache shards/fragments immutably. They are **RECOMMENDED for builders** and **invisible to
readers** (which only follow `file`/`frag`). They are **not** required for a valid index.

- **Content-addressed filenames.** Name each shard/fragment by a hash of its content, so
  unchanged content keeps its name. The reference uses the lowercase hex of the **first 8 bytes
  of the SHA-256** of the file's *uncompressed canonical JSON* (16 hex chars), as
  `index/<hash>.json.gz` and `fragment/<hash>.json`. Hashing the *uncompressed* bytes keeps names
  stable even if the gzip encoder differs.
- **Stable document IDs.** Use an ID that does not change when unrelated documents are added or
  removed (colophon uses the page URL). Postings then stay byte-identical except where their
  terms actually changed.
- **Stable shard boundaries.** First-character bucketing (§5) means new vocabulary lands in an
  existing shard rather than reshuffling boundaries.
- **Canonical JSON.** To reproduce byte-identical files, serialize with object keys sorted by
  UTF-8 byte order, no insignificant whitespace, and shortest round-trip number formatting (the
  behavior of Go's `encoding/json`). The manifest is the only file expected to change every build.
- **Deterministic gzip.** Compress shards with a fixed level and a header carrying no timestamp
  or filename, so identical input yields identical bytes.

A builder in another language that does not care about incrementality **MAY** ignore all of the
above and emit any unique `file`/`frag` paths; the index is still valid.

---

## 9. Cross-language notes

### 9.1 Encoding
All files are UTF-8. JSON per RFC 8259.

### 9.2 Numbers
`avgdl` and `bm25.*` are JSON numbers read as IEEE-754 doubles. `len`, `docCount`, and term
frequencies are non-negative integers.

### 9.3 String comparison
Shard range containment (§5) and the tie-break (§7.4) compare strings by **Unicode scalar value
(code point)**. For the Basic Multilingual Plane this coincides with UTF-8 byte order and UTF-16
code-unit order, so most implementations agree by default. Implementations that must handle
astral-plane first characters or IDs **SHOULD** compare by code point explicitly (note that
JavaScript's default `<` compares UTF-16 code units, which diverges from code-point order for
unpaired/astral cases).

### 9.4 First character
"First character" (§5) means the first Unicode scalar value of the term, not the first UTF-16
code unit or byte.

---

## 10. The `fuzzy` index type (optional)

An index **MAY** include a **trigram index** for typo-tolerant matching. It's present iff the
manifest has a non-empty **`trigrams`** array (same `{lo,hi,file}` shard descriptors as `shards`,
bucketed by the trigram's first character). Each trigram shard is a gzipped JSON object mapping a
**trigram → the sorted terms that contain it**:

```json
{ "$ti": ["tigris"], "tig": ["tigris"], "igr": ["tigris"] }
```

**Trigrams of a term** (the shared contract — a builder inverts term→trigrams; the query side
intersects a token's trigrams against the index): pad the term with a `$` sentinel (outside the
analyzer's letter/number alphabet), then take every distinct length-3 window of **code points** of
`$term$`. `go` → `["$go","go$"]`; `tigris` → `["$ti","tig","igr","gri","ris","is$"]`.

**Edit distance** is rune-wise Levenshtein (insert/delete/substitute), and the **budget** is
`maxEditDist(token) = 1 if len ≤ 4 else 2`. Both **MUST** match the reference (and the JS reader),
guarded by test vectors like the analyzer.

**Query (extends §7).** Fuzzy is a **fallback per token**, so clean queries stay exact: for a query
token with *no* exact/prefix match (and only then), gather candidate terms = the union of
`trigrams[g]` over the token's trigrams `g`, keep those with `levenshtein(token, term) ≤
maxEditDist(token)`, and add them to the matched-term set — scored by their lexical postings
exactly like any other matched term (§7.4). Because Go (in-memory) and the JS reader (trigram
shards) derive the *same* candidate set and apply the *same* distance budget, fuzzy rankings match.

## 11. The `semantic` index type (future)

Reserved: per-document/chunk embedding vectors for hybrid recall, with an *embedder-parity*
contract analogous to the analyzer contract (build-side and query-side embedder must be the same
model). Not yet defined. A reader that doesn't implement an optional type ignores its manifest
section and behaves as lexical-only.

---

## 12. Conformance test vectors

Two committed fixtures let any implementation self-check:

- [`testdata/analyzer.json`](testdata/analyzer.json) — analyzer cases (§6). A conformant analyzer
  reproduces every `out`.
- [`testdata/fixture/`](testdata/fixture/) — a complete example index (`manifest.json`,
  `index/`, `fragment/`) plus [`expected.json`](testdata/fixture/expected.json) mapping queries to
  their ranked results (`id`, `url`, `title`, `score`). A conformant reader, run over the fixture,
  reproduces each query's result order and scores (to within 1e-9).

The reference Node reader test [`search.test.mjs`](search.test.mjs) demonstrates exercising both
fixtures and is a template for a conformance harness in another language.
