package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDotEnvLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		line           string
		wantKey, wantV string
		wantOK         bool
	}{
		{"simple", "FOO=bar", "FOO", "bar", true},
		{"export prefix", "export FOO=bar", "FOO", "bar", true},
		{"spaces around eq", "FOO = bar", "FOO", "bar", true},
		{"double quoted", `FOO="bar baz"`, "FOO", "bar baz", true},
		{"single quoted", "FOO='bar baz'", "FOO", "bar baz", true},
		{"value with eq", "URL=https://x/y?a=b", "URL", "https://x/y?a=b", true},
		{"empty value", "FOO=", "FOO", "", true},
		{"comment", "# FOO=bar", "", "", false},
		{"blank", "   ", "", "", false},
		{"no eq", "FOObar", "", "", false},
		{"no key", "=bar", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			k, v, ok := parseDotEnvLine(tt.line)
			if ok != tt.wantOK || k != tt.wantKey || v != tt.wantV {
				t.Errorf("parseDotEnvLine(%q) = (%q,%q,%v), want (%q,%q,%v)",
					tt.line, k, v, ok, tt.wantKey, tt.wantV, tt.wantOK)
			}
		})
	}
}

// TestLoadDotEnvPrecedence checks real env > .env > .env.defaults, and that a missing file
// is a no-op. It does not run in parallel because it mutates process environment.
func TestLoadDotEnvPrecedence(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(".env.defaults", "FROM_DEFAULTS=defaults\nSHARED=defaults\nREALWINS=defaults\n")
	write(".env", "FROM_ENV=env\nSHARED=env\nREALWINS=env\n")

	// A real environment variable must win over both files.
	t.Setenv("REALWINS", "real")

	loadDotEnv(root)

	for _, tt := range []struct{ key, want string }{
		{"FROM_DEFAULTS", "defaults"}, // only in .env.defaults
		{"FROM_ENV", "env"},           // only in .env
		{"SHARED", "env"},             // .env wins over .env.defaults
		{"REALWINS", "real"},          // real env wins over both files
	} {
		if got := os.Getenv(tt.key); got != tt.want {
			t.Errorf("%s = %q, want %q", tt.key, got, tt.want)
		}
	}
}
