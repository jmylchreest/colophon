// Package local implements the "local" publisher: it copies the built tree to a
// directory on disk.
package local

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/core"
	"github.com/jmylchreest/colophon/internal/publish"
)

func init() { publish.Register("local", New) }

// New builds a local publisher. "path" (relative to root) is the destination dir.
func New(root string, cfg config.PublisherConfig) (core.Publisher, error) {
	path, _ := cfg.Settings["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("local publisher %q: missing 'path'", cfg.ID)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	return &Publisher{id: cfg.ID, dest: filepath.Clean(path)}, nil
}

type Publisher struct {
	id   string
	dest string
}

func (p *Publisher) ID() string     { return p.id }
func (p *Publisher) Driver() string { return "local" }

// Plan schedules every file for upload. Manifest-based incremental diffing
// (skip-unchanged, delete-removed) is a later M1+ refinement.
func (p *Publisher) Plan(ctx context.Context, tree fs.FS) ([]core.Change, error) {
	var changes []core.Change
	err := fs.WalkDir(tree, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		changes = append(changes, core.Change{Path: path, Op: core.OpUpload})
		return nil
	})
	return changes, err
}

func (p *Publisher) Apply(ctx context.Context, tree fs.FS, changes []core.Change) (core.Result, error) {
	var res core.Result
	for _, c := range changes {
		if c.Op != core.OpUpload {
			continue
		}
		b, err := fs.ReadFile(tree, c.Path)
		if err != nil {
			return res, err
		}
		out := filepath.Join(p.dest, filepath.FromSlash(c.Path))
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return res, err
		}
		if err := os.WriteFile(out, b, 0o644); err != nil {
			return res, err
		}
		res.Uploaded++
		res.Bytes += int64(len(b))
	}
	res.Total = res.Uploaded
	return res, nil
}

func (p *Publisher) Invalidate(ctx context.Context, paths []string) error { return nil }
