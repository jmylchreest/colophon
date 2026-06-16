package render

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed themes
var builtinThemes embed.FS

// DefaultTheme is used when a site/environment names no theme.
const DefaultTheme = "default"

// BuiltinThemes lists the embedded theme names, sorted.
func BuiltinThemes() []string {
	entries, _ := fs.ReadDir(builtinThemes, "themes")
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names
}

// ExtractTheme copies a built-in theme's files to destDir (e.g. themes/<name>/ in a
// project) so the author can edit them; the on-disk copy then overrides the built-in.
// It returns the slash-relative paths written.
func ExtractTheme(name, destDir string) ([]string, error) {
	sub, err := fs.Sub(builtinThemes, "themes/"+name)
	if err != nil {
		return nil, err
	}
	if _, err := fs.Stat(sub, "page.html"); err != nil {
		return nil, fmt.Errorf("unknown built-in theme %q (have: %s)", name, strings.Join(BuiltinThemes(), ", "))
	}
	var written []string
	err = fs.WalkDir(sub, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(sub, p)
		if err != nil {
			return err
		}
		out := filepath.Join(destDir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(out, b, 0o644); err != nil {
			return err
		}
		written = append(written, p)
		return nil
	})
	return written, err
}

// themeSource reads theme files, preferring an on-disk override over the built-in theme.
// Built-in themes live under internal/render/themes/<name>/ (embedded); a project may
// override any file by placing it at themes/<name>/ in the project root.
type themeSource struct {
	diskDir  string
	embedded fs.FS // the chosen built-in theme, or the default if the name is unknown
}

func newThemeSource(root, theme string) (*themeSource, error) {
	if theme == "" {
		theme = DefaultTheme
	}
	emb, err := builtinTheme(theme)
	if err != nil {
		return nil, err
	}
	ts := &themeSource{embedded: emb}
	// A project theme dir overrides the built-in per-file (and may add files). It is used
	// for both a named built-in's overrides and an entirely custom theme.
	dir := filepath.Join(root, "themes", theme)
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		ts.diskDir = dir
	}
	return ts, nil
}

// builtinTheme returns the embedded FS rooted at the named theme, falling back to the
// default theme when the name is not a built-in (so a project-only theme name still has
// the default's files to inherit from).
func builtinTheme(name string) (fs.FS, error) {
	if sub, err := fs.Sub(builtinThemes, "themes/"+name); err == nil {
		if _, err := fs.Stat(sub, "page.html"); err == nil {
			return sub, nil
		}
	}
	return fs.Sub(builtinThemes, "themes/"+DefaultTheme)
}

func (t *themeSource) read(name string) ([]byte, error) {
	if t.diskDir != "" {
		if b, err := os.ReadFile(filepath.Join(t.diskDir, filepath.FromSlash(name))); err == nil {
			return b, nil
		}
	}
	return fs.ReadFile(t.embedded, name)
}

// staticAssets lists the theme's non-template files (everything but *.html), slash-paths
// relative to the theme root, unioning the built-in theme with any on-disk override so a
// project can add assets (fonts, extra CSS). The build copies each verbatim to the output.
func (t *themeSource) staticAssets() ([]string, error) {
	seen := map[string]struct{}{}
	add := func(p string) {
		if !strings.HasSuffix(p, ".html") {
			seen[p] = struct{}{}
		}
	}
	if err := fs.WalkDir(t.embedded, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		add(p)
		return nil
	}); err != nil {
		return nil, err
	}
	if t.diskDir != "" {
		_ = filepath.WalkDir(t.diskDir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if rel, err := filepath.Rel(t.diskDir, p); err == nil {
				add(filepath.ToSlash(rel))
			}
			return nil
		})
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out, nil
}
