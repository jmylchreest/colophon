package core

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestGlossaryEntryUnmarshal(t *testing.T) {
	src := `
PCM: Pulse-Code Modulation — uncompressed digital audio.
IPA:
  definition: International Phonetic Alphabet.
  links:
    - { label: Chart, url: "https://example.com/chart" }
    - { label: Wiki, url: "https://example.com/wiki" }
`
	var g map[string]GlossaryEntry
	if err := yaml.Unmarshal([]byte(src), &g); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Bare string form: definition only, no links.
	if got := g["PCM"].Definition; got != "Pulse-Code Modulation — uncompressed digital audio." {
		t.Errorf("PCM definition = %q", got)
	}
	if len(g["PCM"].Links) != 0 {
		t.Errorf("PCM should have no links, got %d", len(g["PCM"].Links))
	}

	// Mapping form: definition + links.
	ipa := g["IPA"]
	if ipa.Definition != "International Phonetic Alphabet." {
		t.Errorf("IPA definition = %q", ipa.Definition)
	}
	if len(ipa.Links) != 2 {
		t.Fatalf("IPA should have 2 links, got %d", len(ipa.Links))
	}
	if ipa.Links[0].Label != "Chart" || ipa.Links[0].URL != "https://example.com/chart" {
		t.Errorf("IPA link[0] = %+v", ipa.Links[0])
	}
}
