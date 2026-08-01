package core

import "strings"

// DefaultLang returns a BCP-47 language tag, or "en" when unset.
func DefaultLang(lang string) string {
	if l := strings.TrimSpace(lang); l != "" {
		return l
	}
	return "en"
}

// PrimarySubtag returns the primary language subtag of a BCP-47 tag, lower-cased (es-MX → es).
func PrimarySubtag(tag string) string {
	t := strings.ToLower(strings.TrimSpace(tag))
	if i := strings.IndexByte(t, '-'); i > 0 {
		return t[:i]
	}
	return t
}

// LangMatches reports whether a post in got satisfies the wanted tag, case-insensitively. A bare
// primary subtag stands for all its variants either way (es ↔ es-MX); two regions (es-MX, es-ES)
// don't match, so a site can address them separately.
func LangMatches(want, got string) bool {
	want, got = strings.ToLower(strings.TrimSpace(want)), strings.ToLower(strings.TrimSpace(got))
	if want == "" || got == "" {
		return false
	}
	if want == got {
		return true
	}
	if !strings.ContainsRune(want, '-') {
		return want == PrimarySubtag(got)
	}
	if !strings.ContainsRune(got, '-') {
		return got == PrimarySubtag(want)
	}
	return false
}
