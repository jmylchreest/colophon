# Design: static search

> Status: **design** · relates to PLAN §8 (search), §9 (publishers). Supersedes the
> `SearchCmd`/`SyncCmd` stubs (`internal/cli/stubs.go`).

Goal: public-site search that is **fully static** (no server, no external service), **low
bandwidth** (never load the whole index into the browser), and **incremental-friendly** (a
content edit rewrites a handful of files, not the index). The same engine powers the
`colophon search` CLI and persona exemplar retrieval.

The design borrows its *architecture* from [Pagefind](https://github.com/Pagefind/pagefind)
(MIT — reviewed, not vendored) — build-from-output, a sharded word index, per-page fragments,
two-stage fetch — but uses **our own format** and a **vanilla-JS reader**, so we own every byte
and track no private binary spec.

## Why not just use Pagefind

Pagefind is excellent but is a Rust/WASM toolchain — bundling it breaks colophon's
single-binary principle, and there is **no Go port of its indexer** (even Hugo, a Go SSG, shells
out to the Rust binary). Targeting Pagefind's on-disk format from Go is worse: it's an internal,
versioned, CBOR layout with an index↔WASM version handshake — we'd be the sole maintainer of a
reverse-engineered emitter chasing every release. So: borrow the ideas, own the format.

## Surfaces and engine sharing

One analyzer + one BM25 definition, three surfaces:

| Surface | Where | Consumes |
|---|---|---|
| **Public site search** | browser, vanilla JS | the static sharded index we emit |
| **`colophon search`** | Go CLI | the same in-memory inverted index, server-side |
| **Persona exemplars** (§8) | Go | the same analyzer + BM25 (zero-config default) |

The browser **never** runs Go/Bleve/WASM — it reads our static files with a small JS scorer.
Bleve's on-disk (Scorch) format has no JS reader and isn't shippable; if Bleve is used at all
it stays **build/CLI-side only**. The reusable core is **hand-rolled and stdlib-only** (see
Packaging) so the inverted index *is* the shared structure across all three surfaces.

The one hard correctness rule: the **analyzer must be identical** in the Go builder and the JS
reader, or a query for "running" won't match an indexed "run". The analyzer is therefore
specified once (below) and implemented twice against that spec.

## Packaging & module boundary

A **separate Go module in this repo**, colophon-branded, wired with a root `go.work`:

```
github.com/jmylchreest/colophon            (the SSG — application module)
github.com/jmylchreest/colophon/search     (the engine — its own go.mod)
```

- **Separate module**, not an in-`colophon` package: gives the engine its **own lean `go.mod`**
  (a `go get …/search` must not drag in pongo2 / goldmark / koanf / go-git) and **independent
  version tags** (`search/vX.Y.Z`).
- **Not under `internal/`** — that's compiler-private; reuse needs a public path.
- **`go.work`** (committed at the repo root, listing `.` and `./search`) so colophon builds
  against the local copy with no published tag, while co-development stays one-repo / one-PR.
- **Hand-rolled, zero-dependency core** — the engine ships its own inverted index + analyzer,
  stdlib only. This is what makes it attractive to adopt. (colophon may still use Bleve
  *separately*, CLI-side, but the published engine does not depend on it.)

The reusable unit is **three artifacts bound by one spec**:

1. **Go builder/query module** (`…/colophon/search`).
2. **JS reader** — a single dependency-free ES module (`search.js`), also publishable to npm.
3. **The format + analyzer spec** (this document) — the real public contract.

### Engine API (source- and FS-agnostic)

```go
package search

// A Doc is anything indexable — the engine knows nothing about colophon pages.
type Doc struct {
    ID    string            // stable, caller-provided (colophon uses the page URL/slug)
    URL   string            // result link
    Title string
    Body  string            // already-extracted plain text
    Meta  map[string]string // shown in the result card; not indexed unless requested
}

type BuildOptions struct {
    Analyzer  Analyzer      // default: SimpleAnalyzer (below)
    ShardFunc ShardFunc     // default: fixed lexical ranges
    BM25      Params        // k1, b
}

// Build writes the static index (manifest + shards + fragments) to dst. dst is an
// abstraction (a dir on disk for most users; colophon routes it through a publisher).
func Build(docs iter.Seq[Doc], dst Writer, opts BuildOptions) (Manifest, error)

// Open mounts an emitted index for server-side querying (the CLI surface).
func Open(fsys fs.FS) (*Index, error)
func (*Index) Search(q string, limit int) ([]Result, error)
```

`Writer` is a minimal `Put(name string, b []byte) error` — the same shape as the publisher
`FileWriter`, so colophon can emit straight through routing, and a standalone user can write to
a directory.

## The analyzer (the contract)

Specified once; implemented identically in Go and JS. **v1 is deliberately trivial** to make
parity self-evident:

1. Unicode NFC normalize.
2. Lowercase (Unicode-aware).
3. Split on any non-(letter|number) run → tokens.
4. Drop tokens of length < 1; **no stop-word list, no stemming** in v1.

That's it — two implementations of one pure function `analyze(string) []string`. A shared
golden-vector test fixture (`testdata/analyzer.json`: input → expected tokens) is run by **both**
the Go and JS test suites, so drift is caught mechanically.

Stemming (e.g. a matched Go+JS Snowball/Porter2 pair) and stop words are **deferred** — added
only as a matched pair, behind a version bump of the analyzer id recorded in the manifest.

## Index format

Emitted as plain static files under a configurable base (default `/_search/`):

```
_search/
  manifest.json                 # the mutable root — small, short-TTL
  index/<range>.<hash>.json.gz  # postings shards — immutable, content-addressed
  fragment/<docid>.<hash>.json  # per-result cards — immutable, content-addressed
```

**`manifest.json`** — routing + scoring constants, loaded once (a few KB):

```json
{
  "v": 1,
  "analyzer": "simple-1",
  "bm25": { "k1": 1.2, "b": 0.75 },
  "docs": 412,
  "avgdl": 680.4,
  "shards": [
    { "lo": "a",  "hi": "cz", "url": "index/a-cz.7c1e9b.json.gz" },
    { "lo": "d",  "hi": "gz", "url": "index/d-gz.2f4a01.json.gz" }
  ],
  "fragments": { "...": "docid → fragment/<docid>.<hash>.json" }
}
```

**A postings shard** — `term → [[docId, termFreq]]` (positions omitted in v1 → no phrase search):

```json
{ "tiger": [[7,1],[88,2]], "tigris": [[7,3]] }
```

DocIds in postings are **small integers** interned from the stable string ID via a table in the
manifest — compact in postings, while the *interning is deterministic from sorted stable IDs*
(see below). Fragments are keyed by the string docId.

**A fragment** — everything needed to render one result, fetched only for shown hits:

```json
{ "url": "/posts/tigris/", "title": "Publishing to Tigris", "excerpt": "…", "meta": {"type":"post"} }
```

## Sharding — fixed lexical ranges

Shards are bucketed by **fixed, stable lexical term ranges** (`a–cz`, `d–gz`, …), **not**
Pagefind's fixed-*count* chunks. Rationale: fixed-count chunks shift their split points as
vocabulary grows, cascading rewrites across many shards; fixed ranges mean a new term lands in
its existing bucket and only that bucket changes. Cost: uneven shard sizes — handled by a
deterministic rule that **sub-splits only over-large ranges** (e.g. `a` → `aa–am`, `an–az`),
which is itself stable given the same vocabulary.

## Determinism & incrementality

The point: an edit should rewrite **as few files as possible**, so the incremental publisher
(content-hash diff + orphan prune, already built) uploads almost nothing and the CDN caches the
rest forever. Five composing rules:

1. **Content-addressed filenames** — every shard and fragment is named by a hash of its bytes.
   Unchanged content → identical name → publisher sees no change → no upload; and the file can be
   served `Cache-Control: immutable, max-age=1y`. The **manifest is the only mutable file** (it
   maps logical keys → current hashes): a *mutable root over an immutable, content-addressed
   tree* (git's model).
2. **Stable doc IDs, never positional.** Postings key off a stable per-doc ID (colophon: the
   page URL). Adding/removing a post must not renumber the others. Integer interning for
   compactness is assigned by **sorted stable-ID order recorded in the manifest** — deterministic
   and stateless (no committed id-map; honors §8 "regenerable, not committed"). *(Note: a pure
   insert still renumbers ints after it; if that churn proves costly we revisit with a
   prev-manifest-seeded allocator. v1 keeps it stateless.)*
3. **Stable shard boundaries** (fixed lexical ranges, above) — vocabulary growth doesn't reshuffle.
4. **Canonical serialization** — sorted keys, stable number formatting, fixed field order, and
   **gzip with mtime=0 + fixed level**. Without this, "unchanged" content re-hashes every build
   (gzip embeds a timestamp by default). This is what makes the content-addressing actually hold.
5. **Postings/presentation split** — volatile display data (excerpt, title styling, meta) lives
   in **fragments**; postings are just `term → [id, tf]`. A cosmetic edit touches one fragment and
   **zero shards**; a body edit touches that fragment plus only the shards for the terms that
   actually changed.

**Inherent churn (accepted):** the manifest (small, by design); hot-term shards (`the`, `and`)
on most edits (a few files, not the index). **Orphans** (superseded content-addressed files) are
removed by the publisher's existing `delete_orphaned`; the manifest is marked `Protected` so it
is never deleted mid-swap.

Typical "edit one post" outcome: **1 new fragment + 1 changed manifest + a few hot-term shards**,
everything else byte-identical and skipped.

## Browser query flow

The whole reader is ~a screen of dependency-free JS:

1. Fetch `manifest.json` once (cache in memory).
2. `analyze()` the query (the shared analyzer).
3. For each term, binary-search `shards` for its range, fetch **only that shard** (dedupe + cache).
4. BM25 over the loaded postings:
   `idf = ln(1 + (N − df + 0.5)/(df + 0.5))`,
   `score += idf · tf·(k1+1) / (tf + k1·(1 − b + b·dl/avgdl))`
   (`N`, `avgdl`, per-shard `df`, per-doc `dl` all come from the manifest/shard).
5. Sort, take top-`limit`, fetch **only those** fragments, render.

Memory at any instant = manifest + the shards the query touched + the visible fragments. Never
the whole index.

## Progressive enhancement & theme integration

Search ships **only when enabled** (like the glossary ships only when used) and is a
**progressive enhancement** (consistent with the raw-block contract): the search box degrades to
a plain link to an archive/index page without JS, and enhances to live search when `search.js`
loads. The `search.js` + CSS are theme/engine-emitted assets; the index files are emitted by the
build (and routable like any other output — so the index can even live on an object store while
HTML is on Pages).

## Semantic search — out of scope here

Lexical only for this engine. Semantic stays **CLI/agent-side** per §8 (local embeddings,
brute-force cosine over `vectors.f32`). In-browser semantic (transformers.js + a ~25–30MB
quantized MiniLM, now feasible) is noted as a **future optional enhancement** layered on top —
not part of v1, and never the default given the model download.

## Key decisions

| Decision | Choice | Rationale |
|---|---|---|
| Build vs runtime | Build-time static index | Fully static, no server (§8) |
| Format | Our own JSON(.gz) | Own every byte; no Pagefind-format/version coupling |
| Browser runtime | Vanilla JS scorer | No WASM; fine at blog/medium scale; ours to maintain |
| Index shape | Sharded inverted (BM25) | Never load the whole index; low bandwidth |
| Sharding | Fixed lexical ranges | Stable boundaries → minimal rewrites on edit |
| Filenames | Content-addressed | Incremental publish + immutable CDN caching |
| Doc IDs | Stable (URL-derived) | Edits don't renumber → postings stay stable |
| Serialization | Canonical, gzip mtime=0 | Identical content → identical bytes (determinism) |
| Analyzer | Simple (no stemming) v1 | Trivial Go/JS parity; golden-vector tested |
| Packaging | Separate module, same repo, `go.work` | Dep isolation + own tags; co-dev stays cheap |
| Engine deps | Hand-rolled, stdlib-only | Adoptable library; Bleve (if any) stays CLI-side |
| Semantic | CLI-side only | Public semantic needs a model at query time (§8) |

## Acceptance criteria

- [ ] `search.Build` emits manifest + shards + fragments to a `Writer`; re-running on unchanged
      input produces **byte-identical** shard/fragment files (determinism).
- [ ] Editing one doc changes only its fragment, the manifest, and the shards for its changed
      terms — all other files byte-identical.
- [ ] A query loads the manifest + only the shards for its terms (verified by fetch count), and
      fetches fragments only for displayed results.
- [ ] BM25 ranking from the JS reader matches the Go `Index.Search` ranking on a shared fixture.
- [ ] The shared analyzer golden-vector fixture passes in **both** Go and JS suites.
- [ ] `colophon search --json` returns ranked results from the same engine (replaces the stub).
- [ ] With JS disabled, the search UI degrades to a working archive link.
- [ ] `go get github.com/jmylchreest/colophon/search` pulls a lean module (no SSG deps).

## Files to create

- `search/go.mod`, root `go.work` — the module + workspace.
- `search/analyzer.go` (+ `testdata/analyzer.json`) — the analyzer spec + golden vectors.
- `search/index.go` — inverted index, BM25, `Build`, sharding, canonical serialization.
- `search/query.go` — `Open` / `Index.Search` (CLI surface).
- `search/format.go` — manifest/shard/fragment types + content-addressing.
- `search/search.js` (+ test) — the browser reader; shares the analyzer + golden vectors.
- colophon side: `internal/build/search.go` (extract page text → `Doc`s → `Build` via a routed
  Writer), wire `SearchCmd` to `search.Open(...).Search`, theme assets + a `search` partial.

## Out of scope (future)

Stemming/stop-words (matched Go+JS pair, analyzer-id bump); positions → phrase/proximity;
filters/facets + sorts; sub-splitting heuristics tuning; in-browser semantic; a published npm
package for `search.js`.
