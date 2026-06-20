// Package render turns templates + page data into HTML behind a pluggable Engine.
// A minimal default theme is embedded; files in themes/<name>/ override it per-file.
package render

// Engine renders theme templates and reads theme assets (e.g. style.css).
type Engine interface {
	Render(name string, ctx map[string]any) (string, error)
	// HasTemplate reports whether the theme provides a template file by this name (across the
	// project override, the theme, and any base theme), so the build can pick a per-page-type
	// template (e.g. "project.html") and fall back to "page.html" when absent.
	HasTemplate(name string) bool
	Asset(name string) ([]byte, error)
	// Assets lists the theme's static files (everything but *.html templates) as
	// slash-separated paths relative to the theme root, for the build to copy verbatim.
	Assets() ([]string, error)
	// Meta returns the theme's optional metadata (theme.yaml), or the zero value if none.
	Meta() ThemeMeta
}
