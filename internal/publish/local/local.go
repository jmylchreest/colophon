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
// "delete_orphaned" (default true) removes dest files no longer in the build tree.
func New(root string, cfg config.PublisherConfig) (core.Publisher, error) {
	path, _ := cfg.Settings["path"].(string)
	if path == "" {
		return nil, fmt.Errorf("local publisher %q: missing 'path'", cfg.ID)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	deleteOrphaned := true
	if v, ok := cfg.Settings["delete_orphaned"].(bool); ok {
		deleteOrphaned = v
	}
	return &Publisher{id: cfg.ID, dest: filepath.Clean(path), deleteOrphaned: deleteOrphaned}, nil
}

type Publisher struct {
	id             string
	dest           string
	deleteOrphaned bool
}

func (p *Publisher) ID() string     { return p.id }
func (p *Publisher) Driver() string { return "local" }

// Deployed walks the destination directory into a path → MD5 manifest. A missing dir (first
// deploy) yields an empty state, not an error.
func (p *Publisher) Deployed(ctx context.Context) (core.State, bool, error) {
	state := core.State{}
	err := filepath.WalkDir(p.dest, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(p.dest, path)
		if err != nil {
			return err
		}
		state[filepath.ToSlash(rel)] = publish.MD5Hex(b)
		return nil
	})
	return state, true, err
}

func (p *Publisher) Hash(name string, b []byte) string { return publish.MD5Hex(b) }
func (p *Publisher) Protected(name string) bool        { return false }

// Put writes a file under dest, creating parent directories.
func (p *Publisher) Put(ctx context.Context, name string, b []byte) error {
	out := filepath.Join(p.dest, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	return os.WriteFile(out, b, 0o644)
}

// Delete removes a file from dest and prunes directories it leaves empty (up to dest).
func (p *Publisher) Delete(ctx context.Context, name string) error {
	out := filepath.Join(p.dest, filepath.FromSlash(name))
	if err := os.Remove(out); err != nil && !os.IsNotExist(err) {
		return err
	}
	for dir := filepath.Dir(out); len(dir) > len(p.dest); dir = filepath.Dir(dir) {
		if err := os.Remove(dir); err != nil {
			break // not empty (or already gone) — stop climbing
		}
	}
	return nil
}

func (p *Publisher) Commit(ctx context.Context, tree fs.FS, plan *core.Plan) (core.Result, error) {
	return publish.CommitFiles(ctx, tree, p, plan, p.deleteOrphaned)
}

func (p *Publisher) Invalidate(ctx context.Context, paths []string) error { return nil }
