package cli

import (
	"strings"
	"testing"
)

func TestUniqueSegment(t *testing.T) {
	existing := map[string]bool{"posts/raft": true, "posts/raft-2": true}
	full := func(s string) string { return "posts/" + s }

	if got := uniqueSegment("intro", full, existing, "counter"); got != "intro" {
		t.Errorf("free segment should be unchanged, got %q", got)
	}
	if got := uniqueSegment("raft", full, existing, "counter"); got != "raft-3" {
		t.Errorf("counter should skip taken suffixes, got %q (want raft-3)", got)
	}

	got := uniqueSegment("raft", full, existing, "hash")
	if got == "raft" || existing[full(got)] {
		t.Errorf("hash should produce an unused segment, got %q", got)
	}
	if !strings.HasPrefix(got, "raft-") {
		t.Errorf("hash suffix should extend the base, got %q", got)
	}
}

func TestFirstCSV(t *testing.T) {
	cases := map[string]string{"": "", "blog": "blog", " a , b ": "a", ",,x": "x"}
	for in, want := range cases {
		if got := firstCSV(in); got != want {
			t.Errorf("firstCSV(%q) = %q, want %q", in, got, want)
		}
	}
}
