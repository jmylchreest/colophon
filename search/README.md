# colophon/search

A standalone, **dependency-free** static search engine: build a sharded, content-addressed BM25
index from a set of documents, query it server-side in Go, or query it in the browser with a tiny
reader that loads only the shards a query touches — no server, no service, no WASM.

It powers [colophon](../README.md)'s on-site search, but knows nothing about colophon: it indexes
any `{ID, URL, Title, Body}` documents, so it's reusable on its own.

- **The contract is specified language-neutrally in [SPEC.md](SPEC.md)** — implement a conformant
  reader or builder in any language from that document alone, and check it against the committed
  test vectors (`testdata/`).
- The design rationale is in [../docs/design/search.md](../docs/design/search.md).

## Go usage

```go
import "github.com/jmylchreest/colophon/search"

docs := []search.Doc{
    {ID: "/a/", URL: "/a/", Title: "Go", Body: "the go programming language"},
    {ID: "/b/", URL: "/b/", Title: "Rust", Body: "a systems programming language"},
}

// Query in memory.
ix, _ := search.NewIndex(docs, search.BuildOptions{})
for _, r := range ix.Search("programming", 10) {
    fmt.Println(r.URL, r.Score)
}

// Or emit a static index for the browser, then re-open it server-side.
man, _ := search.Build(docs, search.DirWriter("public/_search"), search.BuildOptions{})
reopened, _ := search.Open(os.DirFS("public/_search"))
_ = man
_ = reopened
```

`Writer` is a one-method sink (`Put(name string, data []byte) error`) so the index can stream to
a directory (`DirWriter`) or straight through a deploy target.

## Browser usage

`search.js` is the dependency-free ES-module reader. Point it at the emitted directory:

```js
import { createReader } from "/_search/search.js";
const reader = createReader({ base: "/_search/" });
const results = await reader.query("programming", 8); // [{ id, url, title, excerpt, meta, score }]
```

It loads `manifest.json` once, fetches only the shards the query needs (gunzipping them in-browser
via `DecompressionStream`), scores BM25, and fetches only the fragments it shows.

## Parity

The Go builder and the JS reader must agree. `analyze()` is specified once (SPEC §6) and tested on
both sides with the same `testdata/analyzer.json` golden vectors; `search.test.mjs` additionally
replays a Go-emitted fixture and asserts identical ranking and scores. Run:

```sh
go test ./...                 # Go engine + regenerates the fixture
node --test search.test.mjs   # analyzer + ranking parity against the fixture
```

## Status

v1 is lexical (BM25). Fuzzy (trigram + Levenshtein) and semantic (embedding vectors) are designed
as additive index types over the same substrate — see SPEC §10 and the design doc.
