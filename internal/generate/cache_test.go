package generate

import (
	"strings"
	"testing"
)

func TestCacheKeyDeterministicAndSensitive(t *testing.T) {
	base := CacheKey("google", "m1", "a fox", "ink style", map[string]string{"aspect": "16:9"})
	if base != CacheKey("google", "m1", "a fox", "ink style", map[string]string{"aspect": "16:9"}) {
		t.Fatal("same inputs must yield the same key")
	}
	// Param ordering must not matter; key is built from sorted params.
	multi := CacheKey("google", "m1", "a fox", "", map[string]string{"aspect": "16:9", "seed": "7"})
	if multi != CacheKey("google", "m1", "a fox", "", map[string]string{"seed": "7", "aspect": "16:9"}) {
		t.Fatal("param order must not change the key")
	}
	for name, key := range map[string]string{
		"prompt":   CacheKey("google", "m1", "a cat", "ink style", map[string]string{"aspect": "16:9"}),
		"model":    CacheKey("google", "m2", "a fox", "ink style", map[string]string{"aspect": "16:9"}),
		"provider": CacheKey("minimax", "m1", "a fox", "ink style", map[string]string{"aspect": "16:9"}),
		"system":   CacheKey("google", "m1", "a fox", "watercolor", map[string]string{"aspect": "16:9"}),
		"params":   CacheKey("google", "m1", "a fox", "ink style", map[string]string{"aspect": "1:1"}),
	} {
		if key == base {
			t.Errorf("changing %s must change the key", name)
		}
	}
}

func TestStem(t *testing.T) {
	n := Stem("google", "gemini", "A Fox, in the Snow!", "", nil)
	if !strings.HasPrefix(n, "a-fox-in-the-snow-") {
		t.Errorf("expected readable slug prefix, got %q", n)
	}
	if strings.ContainsRune(n, '.') {
		t.Errorf("stem must not carry an extension, got %q", n)
	}
}

func TestImageStemMergesDefaults(t *testing.T) {
	s := Settings{Provider: "google", Model: "m1", Defaults: map[string]string{"aspect": "16:9"}}
	// A ref with no params inherits the default aspect, so it must match an explicit one.
	if s.ImageStem("a fox", "", nil) != s.ImageStem("a fox", "", map[string]string{"aspect": "16:9"}) {
		t.Fatal("default param should fold into the cache name")
	}
	// A per-ref param overrides the default, producing a distinct image.
	if s.ImageStem("a fox", "", nil) == s.ImageStem("a fox", "", map[string]string{"aspect": "1:1"}) {
		t.Fatal("overriding a default must change the name")
	}
	// The system prompt is part of the identity.
	if s.ImageStem("a fox", "", nil) == s.ImageStem("a fox", "ink style", nil) {
		t.Fatal("system prompt must change the name")
	}
	r := s.Request("a fox", "ink style", nil)
	if r.Params["aspect"] != "16:9" {
		t.Errorf("Request should carry merged default, got %q", r.Params["aspect"])
	}
	if r.System != "ink style" {
		t.Errorf("Request should carry the system prompt, got %q", r.System)
	}
}
