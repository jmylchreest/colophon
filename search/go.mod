// Module colophon/search is a standalone, dependency-free static search engine: it builds a
// sharded, content-addressed lexical index from a set of documents and answers BM25 queries.
// It is its own module (not an internal package) so it can be adopted without pulling in the
// colophon SSG's dependency graph. See ../docs/design/search.md.
module github.com/jmylchreest/colophon/search

go 1.26.4
