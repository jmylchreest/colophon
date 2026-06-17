// Package search is a standalone, dependency-free static search engine. It builds a sharded,
// content-addressed BM25 index from a set of documents (Build) that a tiny browser client can
// query by loading only the shards a query touches, and answers the same queries server-side
// (Open + Index.Search). It is intentionally SSG-agnostic and stdlib-only so it can be adopted
// without pulling in a site generator's dependencies. See ../docs/design/search.md.
package search
