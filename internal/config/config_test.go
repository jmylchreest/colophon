package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeProject(t *testing.T, cfg, persona string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ConfigFile), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if persona != "" {
		dir := filepath.Join(root, "personas")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "p.yaml"), []byte(persona), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const validPersona = `
id: default
name: House voice
style:
  guide: Plain and concise.
`

func TestLoadValid(t *testing.T) {
	cfg := `
sites:
  - id: main
    title: T
    base_url: "{env:COLOPHON_T_URL:-http://localhost}"
    theme: default
    personas: [default]
    publishers: [local]
publishers:
  - id: local
    driver: local
    path: ./public
`
	c, err := Load(writeProject(t, cfg, validPersona))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Sites) != 1 || len(c.Publishers) != 1 || len(c.Personas) != 1 {
		t.Fatalf("unexpected counts: %d sites, %d publishers, %d personas", len(c.Sites), len(c.Publishers), len(c.Personas))
	}
	if got := c.Sites[0].BaseURL; got != "http://localhost" {
		t.Errorf("base_url = %q, want interpolated default", got)
	}
	if got := c.Publishers[0].Settings["path"]; got != "./public" {
		t.Errorf("inline driver setting path = %v, want ./public", got)
	}
}

func TestValidateErrors(t *testing.T) {
	tests := []struct {
		name    string
		cfg     string
		persona string
	}{
		{
			name: "unknown publisher ref",
			cfg: `
sites:
  - id: main
    publishers: [missing]
    personas: [default]
publishers:
  - id: local
    driver: local
`,
			persona: validPersona,
		},
		{
			name: "unknown persona ref",
			cfg: `
sites:
  - id: main
    publishers: [local]
    personas: [ghost]
publishers:
  - id: local
    driver: local
`,
			persona: validPersona,
		},
		{
			name: "publisher missing driver",
			cfg: `
sites: []
publishers:
  - id: local
`,
		},
		{
			name: "persona missing id",
			cfg: `
sites: []
publishers: []
`,
			persona: `
name: Voice with no id
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Load(writeProject(t, tt.cfg, tt.persona)); err == nil {
				t.Error("expected validation error, got nil")
			}
		})
	}
}
