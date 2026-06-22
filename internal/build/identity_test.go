package build

import (
	"strings"
	"testing"
)

func TestRelMeLinks(t *testing.T) {
	got := relMeLinks([]string{"https://github.com/me", "  ", "https://m.example/@me?a=1&b=2"})
	if !strings.Contains(got, `<link rel="me" href="https://github.com/me">`) {
		t.Errorf("missing github rel=me in %q", got)
	}
	if !strings.Contains(got, "a=1&amp;b=2") {
		t.Errorf("ampersand not escaped in %q", got)
	}
	if strings.Count(got, "<link") != 2 { // blank URL skipped
		t.Errorf("expected 2 links (blank dropped), got %q", got)
	}

	// No urls → empty (no spurious tag).
	if relMeLinks(nil) != "" {
		t.Error("nil urls should yield empty")
	}
	if relMeLinks([]string{"  ", ""}) != "" {
		t.Error("all-blank urls should yield empty")
	}
}
