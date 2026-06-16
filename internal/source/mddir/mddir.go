// Package mddir implements the "md-dir" source: a directory of markdown files. It is
// the default source (a project's content/ folder) and the lowest common denominator
// other filesystem sources build on.
package mddir

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/source"
	"github.com/jmylchreest/colophon/markdown"
)

func init() { source.Register("md-dir", New) }

// New builds an md-dir source. "path" (relative to root) defaults to "content".
func New(root string, cfg config.SourceConfig) (core.Source, error) {
	path, _ := cfg.Settings["path"].(string)
	if path == "" {
		path = "content"
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	return &Source{id: cfg.ID, dir: path}, nil
}

// Source reads markdown documents from a directory tree.
type Source struct {
	id  string
	dir string
}

func (s *Source) ID() string     { return s.id }
func (s *Source) Driver() string { return "md-dir" }

func (s *Source) Open(ctx context.Context, ref string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(s.dir, filepath.FromSlash(ref)))
}

// Documents walks the directory and parses every .md file. A missing directory yields
// no documents rather than an error, so an empty project still builds.
func (s *Source) Documents(ctx context.Context) ([]core.Content, error) {
	return Walk(s.dir, nil)
}

// Walk parses every .md file under dir into a Content, applying keep (when non-nil) to
// decide inclusion by frontmatter. It is exported so other filesystem sources (e.g.
// obsidian) can reuse the traversal and add their own filtering.
func Walk(dir string, keep func(markdown.Frontmatter) bool) ([]core.Content, error) {
	var docs []core.Content
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Split frontmatter first and apply keep before copying the body, so a note rejected
		// on frontmatter (e.g. an Obsidian publish gate) skips the body-string allocation.
		fm, body, err := markdown.ParseFrontmatter(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if keep != nil && !keep(fm) {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		docs = append(docs, core.Content{
			Document:   markdown.Document{Frontmatter: fm, Body: string(body)},
			SourcePath: filepath.ToSlash(rel),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return docs, nil
}
