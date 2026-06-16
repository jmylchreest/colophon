package local

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/jmylchreest/colophon/internal/config"
	"github.com/jmylchreest/colophon/internal/publish"
)

func newLocal(t *testing.T, deleteOrphaned bool) (*Publisher, string) {
	t.Helper()
	dest := t.TempDir()
	pub, err := New("", config.PublisherConfig{ID: "local", Driver: "local",
		Settings: map[string]any{"path": dest, "delete_orphaned": deleteOrphaned}})
	if err != nil {
		t.Fatal(err)
	}
	return pub.(*Publisher), dest
}

func TestLocalIncrementalAndDelete(t *testing.T) {
	p, dest := newLocal(t, true)
	tree := fstest.MapFS{
		"index.html":         {Data: []byte("home")},
		"posts/a/index.html": {Data: []byte("post a")},
	}
	res, err := publish.Run(context.Background(), tree, p)
	if err != nil {
		t.Fatal(err)
	}
	if res.Uploaded != 2 || res.Total != 2 {
		t.Errorf("first run: uploaded=%d total=%d, want 2/2", res.Uploaded, res.Total)
	}
	if b, _ := os.ReadFile(filepath.Join(dest, "posts/a/index.html")); string(b) != "post a" {
		t.Errorf("file not written, got %q", b)
	}

	// Unchanged re-run transfers nothing.
	if res, _ = publish.Run(context.Background(), tree, p); res.Uploaded != 0 || res.Deleted != 0 {
		t.Errorf("re-run: uploaded=%d deleted=%d, want 0/0", res.Uploaded, res.Deleted)
	}

	// Edit one file → only it uploads.
	tree["index.html"] = &fstest.MapFile{Data: []byte("home v2")}
	if res, _ = publish.Run(context.Background(), tree, p); res.Uploaded != 1 || res.Deleted != 0 {
		t.Errorf("after edit: uploaded=%d deleted=%d, want 1/0", res.Uploaded, res.Deleted)
	}

	// Remove a file → orphan deleted and its now-empty dir pruned.
	delete(tree, "posts/a/index.html")
	if res, _ = publish.Run(context.Background(), tree, p); res.Deleted != 1 {
		t.Errorf("after removal: deleted=%d, want 1", res.Deleted)
	}
	if _, err := os.Stat(filepath.Join(dest, "posts/a/index.html")); !os.IsNotExist(err) {
		t.Error("orphaned file was not deleted")
	}
	if _, err := os.Stat(filepath.Join(dest, "posts")); !os.IsNotExist(err) {
		t.Error("emptied directory was not pruned")
	}
}

func TestLocalDeleteOrphanedFalse(t *testing.T) {
	p, dest := newLocal(t, false)
	tree := fstest.MapFS{"a.html": {Data: []byte("a")}, "b.html": {Data: []byte("b")}}
	_, _ = publish.Run(context.Background(), tree, p)
	delete(tree, "b.html")
	res, _ := publish.Run(context.Background(), tree, p)
	if res.Deleted != 0 {
		t.Errorf("delete_orphaned=false should delete nothing, got %d", res.Deleted)
	}
	if _, err := os.Stat(filepath.Join(dest, "b.html")); err != nil {
		t.Error("b.html should be preserved when delete_orphaned=false")
	}
}
