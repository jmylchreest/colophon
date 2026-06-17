package search

import (
	"strings"
	"unicode"
)

// SimpleAnalyzerID identifies the v1 tokenization rules. It is recorded in the emitted manifest
// so a browser reader can refuse an index built by incompatible rules. Bump it whenever the
// rules change (e.g. when NFC normalization or stemming is added) — see docs/design/search.md.
const SimpleAnalyzerID = "simple-1"

// Analyze tokenizes text into search terms using the v1 "simple-1" rules: Unicode-aware
// lowercase, then split on any run of non-(letter|number) characters. No NFC normalization, no
// stop-words, no stemming — deliberately trivial so the Go and JavaScript implementations stay
// equivalent (guarded by the shared testdata/analyzer.json golden vectors).
//
// The single correctness rule for the whole engine: the analyzer used to build an index and the
// analyzer used to query it must agree, or a search for "running" won't match an indexed "run".
// Keeping v1 this simple makes that parity self-evident.
func Analyze(text string) []string {
	lower := strings.ToLower(text)
	return strings.FieldsFunc(lower, isSeparator)
}

// isSeparator reports whether r breaks a token: anything that is not a letter or a number. This
// is the exact rule the JS reader mirrors with /[^\p{L}\p{N}]+/u.
func isSeparator(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsNumber(r)
}
