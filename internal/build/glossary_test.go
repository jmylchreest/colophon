package build

import (
	"encoding/json"
	"testing"

	"github.com/jmylchreest/colophon/internal/core"
)

// TestEmitGlossaryWireShape pins the published glossary.json shape: a term maps to {def, links},
// and an entry with no links omits the links key (so the flat case stays compact).
func TestEmitGlossaryWireShape(t *testing.T) {
	gloss := map[string]core.GlossaryEntry{
		"PCM": {Definition: "Pulse-Code Modulation."},
		"IPA": {
			Definition: "International Phonetic Alphabet.",
			Links:      []core.GlossaryLink{{Label: "Chart", URL: "https://example.com/chart"}},
		},
	}

	files := map[string][]byte{}
	emitted, err := emitGlossary(func(name string, b []byte) error { files[name] = b; return nil }, gloss)
	if err != nil || !emitted {
		t.Fatalf("emitGlossary emitted=%v err=%v", emitted, err)
	}

	data, ok := files[glossaryData]
	if !ok {
		t.Fatalf("no %s written", glossaryData)
	}
	var got map[string]core.GlossaryEntry
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("glossary.json invalid: %v", err)
	}
	if got["PCM"].Definition != "Pulse-Code Modulation." || len(got["PCM"].Links) != 0 {
		t.Errorf("PCM round-trip = %+v", got["PCM"])
	}
	if len(got["IPA"].Links) != 1 || got["IPA"].Links[0].URL != "https://example.com/chart" {
		t.Errorf("IPA links round-trip = %+v", got["IPA"].Links)
	}

	// A link-less entry must not serialise a links key (kept compact via omitempty).
	var probe map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		t.Fatalf("probe unmarshal: %v", err)
	}
	if _, has := probe["PCM"]["links"]; has {
		t.Errorf("link-less entry should omit \"links\": %s", data)
	}
}
