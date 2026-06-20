package build

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// acronymReplacer expands acronym glossary terms to their spoken form in TTS text (e.g. "SSH"
// → "Secure Shell"), so they're read as words rather than spelled out letter by letter. It is
// nil when the glossary holds no qualifying acronyms.
type acronymReplacer struct {
	re *regexp.Regexp
	m  map[string]string
}

// newAcronymReplacer builds a replacer from the glossary, keeping only entries that look like
// acronym expansions (see isAcronymExpansion). Returns nil when none qualify.
func newAcronymReplacer(glossary map[string]string) *acronymReplacer {
	m := map[string]string{}
	for term, def := range glossary {
		t, d := strings.TrimSpace(term), strings.TrimSpace(def)
		if isAcronymExpansion(t, d) {
			m[t] = d
		}
	}
	if len(m) == 0 {
		return nil
	}
	terms := make([]string, 0, len(m))
	for k := range m {
		terms = append(terms, regexp.QuoteMeta(k))
	}
	sort.Strings(terms)
	// Whole-word, case-sensitive: only the upper-case acronym form is expanded, so a lower-case
	// command like "ssh" is left untouched.
	return &acronymReplacer{re: regexp.MustCompile(`\b(?:` + strings.Join(terms, "|") + `)\b`), m: m}
}

// expand replaces acronyms in spoken text with their expansion.
func (a *acronymReplacer) expand(s string) string {
	if a == nil {
		return s
	}
	return a.re.ReplaceAllStringFunc(s, func(m string) string {
		if e, ok := a.m[m]; ok {
			return e
		}
		return m
	})
}

// isAcronymExpansion decides whether a glossary entry should be read as its expansion in
// speech. The term must be an acronym shape (all upper-case, ≥2 letters, no spaces) and the
// definition a short, single-line, Title-Case multi-word phrase whose letters contain the
// acronym as a subsequence — so "SSH"→"Secure Shell" and "DDD"→"Domain Driven Design" qualify,
// but "Rust" (not all-caps) and descriptive sentences do not.
func isAcronymExpansion(term, def string) bool {
	if term == "" || strings.ContainsAny(term, " \t\n") || term != strings.ToUpper(term) {
		return false
	}
	letters := 0
	for _, r := range term {
		switch {
		case unicode.IsLetter(r):
			letters++
		case unicode.IsDigit(r):
		default:
			return false
		}
	}
	if letters < 2 {
		return false
	}
	if strings.ContainsRune(def, '\n') || len(def) > 60 || !looksLikeExpansion(def) {
		return false
	}
	return isSubsequence(strings.ToLower(onlyLetters(term)), strings.ToLower(onlyLetters(def)))
}

var expansionSmallWords = map[string]bool{
	"a": true, "an": true, "the": true, "of": true, "and": true,
	"for": true, "to": true, "in": true, "on": true, "or": true,
}

// looksLikeExpansion reports whether def reads like a Title-Case multi-word name (each word
// capitalised, bar small connectors) rather than a lower-case description.
func looksLikeExpansion(def string) bool {
	words := strings.Fields(def)
	if len(words) < 2 || len(words) > 8 {
		return false
	}
	capped := 0
	for _, w := range words {
		r := []rune(w)
		if unicode.IsUpper(r[0]) {
			capped++
			continue
		}
		if !expansionSmallWords[strings.ToLower(w)] {
			return false // a non-connector lower-case word → a description, not an expansion
		}
	}
	return capped >= 2
}

func isSubsequence(sub, s string) bool {
	i := 0
	for j := 0; i < len(sub) && j < len(s); j++ {
		if sub[i] == s[j] {
			i++
		}
	}
	return i == len(sub)
}

func onlyLetters(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
