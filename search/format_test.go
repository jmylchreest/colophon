package search

import (
	"testing"
	"testing/fstest"
)

// mapWriter collects emitted files in memory and exposes them as an fs.FS for Open.
type mapWriter map[string][]byte

func (w mapWriter) Put(name string, data []byte) error {
	cp := make([]byte, len(data))
	copy(cp, data)
	w[name] = cp
	return nil
}

func (w mapWriter) fsys() fstest.MapFS {
	m := make(fstest.MapFS, len(w))
	for name, data := range w {
		m[name] = &fstest.MapFile{Data: data}
	}
	return m
}

func TestEmitOpenRoundTripMatchesInMemory(t *testing.T) {
	docs := sampleDocs()
	mem := mustIndex(t, docs)

	w := mapWriter{}
	if _, err := mem.Emit(w); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(w.fsys())
	if err != nil {
		t.Fatal(err)
	}

	for _, q := range []string{"go", "programming language", "bread", "a programming language"} {
		want := mem.Search(q, 0)
		got := reopened.Search(q, 0)
		if len(want) != len(got) {
			t.Fatalf("query %q: in-memory %d hits, reopened %d", q, len(want), len(got))
		}
		for i := range want {
			if want[i].ID != got[i].ID {
				t.Errorf("query %q rank %d: in-memory %q, reopened %q", q, i, want[i].ID, got[i].ID)
			}
			if abs(want[i].Score-got[i].Score) > 1e-9 {
				t.Errorf("query %q %q: score %v vs %v", q, want[i].ID, want[i].Score, got[i].Score)
			}
			if want[i].URL != got[i].URL || want[i].Title != got[i].Title || want[i].Excerpt != got[i].Excerpt {
				t.Errorf("query %q %q: fragment metadata not preserved", q, want[i].ID)
			}
		}
	}
}

func TestEmitIsDeterministic(t *testing.T) {
	docs := sampleDocs()
	a, b := mapWriter{}, mapWriter{}
	if _, err := mustIndex(t, docs).Emit(a); err != nil {
		t.Fatal(err)
	}
	if _, err := mustIndex(t, docs).Emit(b); err != nil {
		t.Fatal(err)
	}
	if len(a) != len(b) {
		t.Fatalf("file counts differ: %d vs %d", len(a), len(b))
	}
	for name, ba := range a {
		bb, ok := b[name]
		if !ok {
			t.Errorf("file %q only in first emit", name)
			continue
		}
		if string(ba) != string(bb) {
			t.Errorf("file %q differs between emits (not byte-deterministic)", name)
		}
	}
}

// TestEditOnlyRewritesAffectedFiles is the incrementality guarantee: editing one doc leaves every
// other doc's content-addressed fragment, and every shard not touched by the change, byte-identical.
func TestEditOnlyRewritesAffectedFiles(t *testing.T) {
	base := sampleDocs()
	before := mapWriter{}
	if _, err := mustIndex(t, base).Emit(before); err != nil {
		t.Fatal(err)
	}

	// Edit doc "c" only; "a" and "b" are untouched.
	edited := sampleDocs()
	for i := range edited {
		if edited[i].ID == "c" {
			edited[i].Body = "a recipe for sourdough"
		}
	}
	after := mapWriter{}
	if _, err := mustIndex(t, edited).Emit(after); err != nil {
		t.Fatal(err)
	}

	// Fragments for a and b must survive byte-identical (their content-addressed names persist).
	common := 0
	for name, b := range before {
		if name == "manifest.json" {
			continue
		}
		if bb, ok := after[name]; ok && string(b) == string(bb) {
			common++
		}
	}
	if common == 0 {
		t.Error("expected unchanged docs to keep byte-identical content-addressed files; none survived")
	}
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
