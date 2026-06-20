package render

import (
	"bytes"
	"fmt"
	"io"

	"github.com/flosch/pongo2/v6"
)

type pongoEngine struct {
	src *themeSource
	set *pongo2.TemplateSet
}

// New returns the default pongo2 engine for a project root and theme name.
func New(root, theme string) (Engine, error) {
	src, err := newThemeSource(root, theme)
	if err != nil {
		return nil, err
	}
	e := &pongoEngine{src: src}
	e.set = pongo2.NewSet("colophon", e)
	return e, nil
}

func (e *pongoEngine) Render(name string, ctx map[string]any) (string, error) {
	tpl, err := e.set.FromFile(name)
	if err != nil {
		return "", fmt.Errorf("load template %s: %w", name, err)
	}
	out, err := tpl.Execute(pongo2.Context(ctx))
	if err != nil {
		return "", fmt.Errorf("render template %s: %w", name, err)
	}
	return out, nil
}

func (e *pongoEngine) HasTemplate(name string) bool {
	return e.src.has(name)
}

func (e *pongoEngine) Asset(name string) ([]byte, error) {
	return e.src.read(name)
}

func (e *pongoEngine) Assets() ([]string, error) {
	return e.src.staticAssets()
}

func (e *pongoEngine) Meta() ThemeMeta {
	return e.src.readMeta()
}

// Abs and Get satisfy pongo2.TemplateLoader. Names are theme-relative, so Abs is identity.
func (e *pongoEngine) Abs(base, name string) string { return name }

func (e *pongoEngine) Get(path string) (io.Reader, error) {
	b, err := e.src.read(path)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(b), nil
}
