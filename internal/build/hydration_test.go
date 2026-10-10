package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmylchreest/colophon/internal/config"
	_ "github.com/jmylchreest/colophon/internal/source/mddir"
)

func TestBlocksLoadHydrationScripts(t *testing.T) {
	for _, theme := range []string{"press", "default"} {
		t.Run(theme, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "content", "posts"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "colophon.yaml"), []byte("sites:\n  - id: main\n    title: Test\n    base_url: https://example.com\n    theme: "+theme+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			md := "---\ntitle: Code\ndate: 2026-01-01\nslides: true\n---\n## Code\n\n```go\nfmt.Println(\"hello\")\n```\n\n## Diagram\n\n```mermaid\ngraph LR; A-->B\n```\n"
			if err := os.WriteFile(filepath.Join(root, "content", "posts", "code.md"), []byte(md), 0o644); err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(root)
			if err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(root, "public")
			if _, err := Run(cfg, Options{OutDir: out}); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"posts/code/index.html", "posts/code/slides/index.html"} {
				b, err := os.ReadFile(filepath.Join(out, filepath.FromSlash(name)))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(b), `tabindex="0"`) || !strings.Contains(string(b), "vendor/highlight/highlight.min.js") {
					t.Errorf("%s must preserve keyboard focus and load the code highlighter", name)
				}
				if !strings.Contains(string(b), "mermaid-init.js") || !strings.Contains(string(b), "vendor/mermaid/mermaid.min.js") {
					t.Errorf("%s must load the Mermaid library and theme-aware renderer", name)
				}
			}
			if _, err := os.Stat(filepath.Join(out, "mermaid-init.js")); err != nil {
				t.Fatal(err)
			}
		})
	}
}
