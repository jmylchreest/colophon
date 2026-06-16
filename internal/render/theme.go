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

// baseMarker is an optional one-line file in a built-in theme naming another built-in
// it inherits from (templates + static assets). It lets a brand theme (e.g. press) reuse
// the default theme's vendored fonts/JS without duplicating ~5MB of embedded bytes.
const baseMarker = "base"

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

// ExtractTheme copies a built-in theme's own files to destDir (e.g. themes/<name>/ in a
// project) so the author can edit them; the on-disk copy then overrides the built-in.
// Inherited base-theme files (e.g. the vendored assets) are left in the binary and still
// resolve at build, so the eject stays small. It returns the slash-relative paths written.
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
		if err != nil || d.IsDir() || p == baseMarker {
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

// themeSource reads theme files, preferring an on-disk override over the built-in theme(s).
// Built-in themes live under internal/render/themes/<name>/ (embedded); a built-in may
// inherit a base via the `base` marker (base-first in layers). A project may override any
// file by placing it at themes/<name>/ in the project root (diskDir, highest precedence).
type themeSource struct {
	diskDir string  // optional on-disk override dir (symlinks resolved when listing assets)
	layers  []fs.FS // base-first, overlay-last: reads check overlay→base, assets union all
}

func newThemeSource(root, theme string) (*themeSource, error) {
	if theme == "" {
		theme = DefaultTheme
	}
	layers, err := embeddedLayers(theme)
	if err != nil {
		return nil, err
	}
	ts := &themeSource{layers: layers}
	// A project theme dir overrides the built-in per-file (and may add files). It is used
	// for both a named built-in's overrides and an entirely custom (on-disk) theme.
	dir := filepath.Join(root, "themes", theme)
	if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
		ts.diskDir = dir
	}
	return ts, nil
}

// embeddedLayers returns the base→overlay FS layers for a theme name. A known built-in
// resolves to its own FS, prefixed by its base theme's layers if it declares one. An
// unknown name falls back to the default theme as a single base layer, so a project-only
// (on-disk) theme still inherits the default's templates and vendored assets.
func embeddedLayers(name string) ([]fs.FS, error) {
	if sub, err := fs.Sub(builtinThemes, "themes/"+name); err == nil {
		if _, err := fs.Stat(sub, "page.html"); err == nil {
			if b, err := fs.ReadFile(sub, baseMarker); err == nil {
				if base := strings.TrimSpace(string(b)); base != "" && base != name {
					baseLayers, err := embeddedLayers(base)
					if err != nil {
						return nil, err
					}
					return append(baseLayers, sub), nil
				}
			}
			return []fs.FS{sub}, nil
		}
	}
	def, err := fs.Sub(builtinThemes, "themes/"+DefaultTheme)
	if err != nil {
		return nil, err
	}
	return []fs.FS{def}, nil
}

// has reports whether the theme provides a file by this name, checking the on-disk override
// then each embedded layer (overlay → base). Used to resolve optional per-page-type templates.
func (t *themeSource) has(name string) bool {
	if t.diskDir != "" {
		if _, err := os.Stat(filepath.Join(t.diskDir, filepath.FromSlash(name))); err == nil {
			return true
		}
	}
	for _, layer := range t.layers {
		if _, err := fs.Stat(layer, name); err == nil {
			return true
		}
	}
	return false
}

func (t *themeSource) read(name string) ([]byte, error) {
	if t.diskDir != "" {
		if b, err := os.ReadFile(filepath.Join(t.diskDir, filepath.FromSlash(name))); err == nil {
			return b, nil
		}
	}
	var lastErr error
	for i := len(t.layers) - 1; i >= 0; i-- { // overlay → base
		if b, err := fs.ReadFile(t.layers[i], name); err == nil {
			return b, nil
		} else {
			lastErr = err
		}
	}
	if lastErr == nil {
		lastErr = fs.ErrNotExist
	}
	return nil, lastErr
}

// staticAssets lists the theme's non-template files (everything but *.html and the base
// marker), slash-paths relative to the theme root, unioning every layer (base→overlay)
// with any on-disk override so a project can add assets (fonts, extra CSS). On-disk
// override dirs may be symlinks (e.g. a fixture pointing at contrib/themes/<name>); they
// are resolved before walking so the real files are found. The build copies each verbatim.
func (t *themeSource) staticAssets() ([]string, error) {
	seen := map[string]struct{}{}
	add := func(p string) {
		if !strings.HasSuffix(p, ".html") && p != baseMarker {
			seen[p] = struct{}{}
		}
	}
	for _, layer := range t.layers {
		if err := fs.WalkDir(layer, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			add(p)
			return nil
		}); err != nil {
			return nil, err
		}
	}
	if t.diskDir != "" {
		base := t.diskDir
		if resolved, err := filepath.EvalSymlinks(t.diskDir); err == nil {
			base = resolved
		}
		_ = filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if rel, err := filepath.Rel(base, p); err == nil {
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
