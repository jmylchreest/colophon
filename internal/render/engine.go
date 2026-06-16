// Package render turns templates + page data into HTML behind a pluggable Engine.
// A minimal default theme is embedded; files in themes/<name>/ override it per-file.
package render

// Engine renders theme templates and reads theme assets (e.g. style.css).
type Engine interface {
	Render(name string, ctx map[string]any) (string, error)
	Asset(name string) ([]byte, error)
	// Assets lists the theme's static files (everything but *.html templates) as
	// slash-separated paths relative to the theme root, for the build to copy verbatim.
	Assets() ([]string, error)
}
