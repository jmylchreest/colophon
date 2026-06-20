package build

import (
	"strings"
	"testing"

	"github.com/jmylchreest/colophon/internal/core"
)

func TestIsAcronymExpansion(t *testing.T) {
	yes := map[string]string{
		"SSH":  "Secure Shell",
		"DDD":  "Domain Driven Design",
		"API":  "Application Programming Interface",
		"HTTP": "Hypertext Transfer Protocol",
		"CPU":  "Central Processing Unit",
	}
	for term, def := range yes {
		if !isAcronymExpansion(term, def) {
			t.Errorf("expected %q/%q to be an acronym expansion", term, def)
		}
	}
	no := map[string]string{
		"Rust": "A systems programming language",                                  // not all-caps
		"ABC":  "a basic concept here",                                            // lower-case description
		"GNU":  "GNU is Not Unix and other things and more words here that go on", // too long
		"X":    "Extended Thing",                                                  // single letter
		"REST": "An architectural style for the web",                              // letters not a subsequence
		"Go":   "Go Programming Language",                                         // not all-caps
	}
	for term, def := range no {
		if isAcronymExpansion(term, def) {
			t.Errorf("expected %q/%q NOT to be an acronym expansion", term, def)
		}
	}
}

func TestAcronymExpansionInSpeech(t *testing.T) {
	glossary := map[string]string{
		"SSH":  "Secure Shell",
		"Rust": "A systems programming language",
	}
	acr := newAcronymReplacer(glossary)
	out := speechText(`<p>Use SSH and Rust today.</p>`, core.SpeechTranscript{}, enTTS, acr)
	if !strings.Contains(out, "Use Secure Shell and Rust today.") {
		t.Errorf("SSH should expand and Rust stay: %s", out)
	}
	// Lower-case command form must not be expanded.
	out2 := speechText(`<p>run ssh now.</p>`, core.SpeechTranscript{}, enTTS, acr)
	if strings.Contains(out2, "Secure Shell") {
		t.Errorf("lower-case ssh should not expand: %s", out2)
	}
	// Toggle off.
	off := false
	out3 := speechText(`<p>Use SSH.</p>`, core.SpeechTranscript{ExpandAcronyms: &off}, enTTS, acr)
	if strings.Contains(out3, "Secure Shell") {
		t.Errorf("expand_acronyms off should leave SSH: %s", out3)
	}
}
