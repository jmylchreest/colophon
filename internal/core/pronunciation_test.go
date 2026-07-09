package core

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestPronunciationDictsUnmarshal(t *testing.T) {
	// Naked scalar → the site-default-language slot.
	var g SpeechGen
	if err := yaml.Unmarshal([]byte("pronunciation_dict: en_GB\n"), &g); err != nil {
		t.Fatal(err)
	}
	if got := g.PronunciationDict; len(got) != 1 || got[""] != "en_GB" {
		t.Errorf("scalar form = %v, want {\"\": en_GB}", got)
	}

	// Map form → per-language, keys lower-cased.
	g = SpeechGen{}
	if err := yaml.Unmarshal([]byte("pronunciation_dict:\n  EN: en_GB\n  es: es_ES\n"), &g); err != nil {
		t.Fatal(err)
	}
	if got := g.PronunciationDict; got["en"] != "en_GB" || got["es"] != "es_ES" {
		t.Errorf("map form = %v", got)
	}

	// A sequence is a config error, not a silent zero value.
	if err := yaml.Unmarshal([]byte("pronunciation_dict: [en_GB]\n"), &SpeechGen{}); err == nil {
		t.Error("sequence form should be rejected")
	}
}

func TestPronunciationDictsFor(t *testing.T) {
	naked := PronunciationDicts{"": "en_GB"}
	// The naked ref follows the site default language: it applies to default-language posts
	// (including regional variants of it) and to no other language.
	if got := naked.For("en", "en"); got != "en_GB" {
		t.Errorf("naked/default = %q, want en_GB", got)
	}
	if got := naked.For("en-US", "en"); got != "en_GB" {
		t.Errorf("naked/regional-variant-of-default = %q, want en_GB", got)
	}
	if got := naked.For("es", "en"); got != "" {
		t.Errorf("naked dict leaked into a non-default language: %q", got)
	}
	// A post with no language resolves as the default language.
	if got := naked.For("", "en"); got != "en_GB" {
		t.Errorf("naked/unset-lang = %q, want en_GB", got)
	}

	m := PronunciationDicts{"en": "en_GB", "es": "es_ES"}
	if got := m.For("es", "en"); got != "es_ES" {
		t.Errorf("map exact = %q, want es_ES", got)
	}
	if got := m.For("es-MX", "en"); got != "es_ES" { // base-language fallback
		t.Errorf("map base fallback = %q, want es_ES", got)
	}
	if got := m.For("fr", "en"); got != "" { // no entry → no dict
		t.Errorf("unlisted language = %q, want none", got)
	}

	if got := (PronunciationDicts)(nil).For("en", "en"); got != "" {
		t.Errorf("nil dicts = %q, want none", got)
	}
}

func TestPronunciationDictsProfileMerge(t *testing.T) {
	base := SpeechGen{
		Provider:          "elevenlabs",
		PronunciationDict: PronunciationDicts{"en": "en_GB", "es": "es_ES"},
		Profiles: map[string]SpeechGen{
			"alt": {PronunciationDict: PronunciationDicts{"es": "dicts/es-alt.yaml"}},
		},
	}
	got, err := base.ResolveProfile("alt")
	if err != nil {
		t.Fatal(err)
	}
	// Per-key merge: the profile overrides es, inherits en.
	if got.PronunciationDict["es"] != "dicts/es-alt.yaml" || got.PronunciationDict["en"] != "en_GB" {
		t.Errorf("merged dicts = %v", got.PronunciationDict)
	}
}
