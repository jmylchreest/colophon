package search

import (
	"os"
	"path/filepath"
)

// DirWriter writes an emitted index to a directory on disk (its string value is the root). It's
// the standalone sink; colophon instead adapts its publisher around the Writer interface so the
// index streams straight to the deploy target.
type DirWriter string

// Put writes data to root/name, creating parent directories. name uses forward slashes.
func (d DirWriter) Put(name string, data []byte) error {
	full := filepath.Join(string(d), filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}
