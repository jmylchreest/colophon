package config

import "testing"

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
