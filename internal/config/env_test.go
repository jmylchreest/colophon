package config

import (
	"strings"
	"testing"
)

func TestEnvRefs(t *testing.T) {
	raw := []byte(`sites:
  - base_url: "{env:BASE_URL}"
publishers:
  - bucket: "{env:R2_BUCKET:-default}"
    project: "{env:CF_PAGES_PROJECT}"
    dup: "{env:BASE_URL}"`)
	if got := strings.Join(envRefs(raw), ","); got != "BASE_URL,CF_PAGES_PROJECT,R2_BUCKET" {
		t.Errorf("envRefs = %q, want sorted unique BASE_URL,CF_PAGES_PROJECT,R2_BUCKET", got)
	}
	if len(envRefs([]byte("no placeholders"))) != 0 {
		t.Error("envRefs should be empty when nothing is referenced")
	}
}

func TestInterpolateEnv(t *testing.T) {
	t.Setenv("COLOPHON_TEST_SET", "value")

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "url: http://x", "url: http://x"},
		{"set var", "url: {env:COLOPHON_TEST_SET}", "url: value"},
		{"unset no default", "url: {env:COLOPHON_TEST_MISSING}", "url: "},
		{"unset with default", "url: {env:COLOPHON_TEST_MISSING:-fallback}", "url: fallback"},
		{"set ignores default", "url: {env:COLOPHON_TEST_SET:-fallback}", "url: value"},
		{"empty default", "url: {env:COLOPHON_TEST_MISSING:-}", "url: "},
		{"multiple", "{env:COLOPHON_TEST_SET}/{env:COLOPHON_TEST_MISSING:-d}", "value/d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(interpolateEnv([]byte(tt.in))); got != tt.want {
				t.Errorf("interpolateEnv(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
