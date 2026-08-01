package cli

import (
	"strings"
	"testing"

	"github.com/jmylchreest/colophon/internal/core"
)

// A post and its translations must each reach only the targets for their own language, so an
// English and a Spanish account don't both receive both versions.
func TestLangTargets(t *testing.T) {
	open := map[string]target{
		"mastodon-en": {SyndicatorConf: core.SyndicatorConf{ID: "mastodon-en"}},
		"mastodon-es": {SyndicatorConf: core.SyndicatorConf{ID: "mastodon-es", Lang: []string{"es"}}},
		"bsky-all":    {SyndicatorConf: core.SyndicatorConf{ID: "bsky-all", Lang: []string{"*"}}},
	}
	all := []string{"mastodon-en", "mastodon-es", "bsky-all"}

	for _, c := range []struct{ lang, keep, other string }{
		{"en", "mastodon-en,bsky-all", "mastodon-es"},
		{"es", "mastodon-es,bsky-all", "mastodon-en"},
		{"fr", "bsky-all", "mastodon-en,mastodon-es"},
	} {
		keep, other := langTargets(all, open, c.lang, "en")
		if got := strings.Join(keep, ","); got != c.keep {
			t.Errorf("lang %q: keep = %q, want %q", c.lang, got, c.keep)
		}
		if got := strings.Join(other, ","); got != c.other {
			t.Errorf("lang %q: other = %q, want %q", c.lang, got, c.other)
		}
	}
}

func TestSyndicateTargets(t *testing.T) {
	allowed := []string{"a", "b"}
	if got := syndicateTargets(nil, allowed); len(got) != 2 {
		t.Errorf("no per-post choice should mean all allowed, got %v", got)
	}
	if got := syndicateTargets([]string{"b", "c"}, allowed); len(got) != 1 || got[0] != "b" {
		t.Errorf("chosen targets must intersect the env's allowed set, got %v", got)
	}
}
