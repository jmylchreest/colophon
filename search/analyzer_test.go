package search

import (
	"encoding/json"
	"os"
	"testing"
)

// TestAnalyzeGoldenVectors runs the shared analyzer fixture that the JS reader's test suite also
// runs, so any drift between the two implementations fails a build on one side or the other.
func TestAnalyzeGoldenVectors(t *testing.T) {
	b, err := os.ReadFile("testdata/analyzer.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		In  string   `json:"in"`
		Out []string `json:"out"`
	}
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		got := Analyze(c.In)
		if !equal(got, c.Out) {
			t.Errorf("Analyze(%q) = %#v, want %#v", c.In, got, c.Out)
		}
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
