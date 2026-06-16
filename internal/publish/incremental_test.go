package publish

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jmylchreest/colophon/internal/core"
)

// fakeBackend is a core.Publisher that records what the planner asks it to do.
type fakeBackend struct {
	have           core.State
	enumerable     bool
	protect        map[string]bool
	deleteOrphaned bool
	puts, dels     []string
}

func (f *fakeBackend) ID() string     { return "fake" }
func (f *fakeBackend) Driver() string { return "fake" }
func (f *fakeBackend) Deployed(ctx context.Context) (core.State, bool, error) {
	return f.have, f.enumerable, nil
}
func (f *fakeBackend) Hash(name string, b []byte) string { return MD5Hex(b) }
func (f *fakeBackend) Protected(name string) bool        { return f.protect[name] }
func (f *fakeBackend) Put(ctx context.Context, name string, b []byte) error {
	f.puts = append(f.puts, name)
	return nil
}
func (f *fakeBackend) Delete(ctx context.Context, name string) error {
	f.dels = append(f.dels, name)
	return nil
}
func (f *fakeBackend) Commit(ctx context.Context, tree fs.FS, plan *core.Plan) (core.Result, error) {
	return CommitFiles(ctx, tree, f, plan, f.deleteOrphaned)
}
func (f *fakeBackend) Invalidate(ctx context.Context, paths []string) error { return nil }

func TestRunDiffsUploadsAndDeletes(t *testing.T) {
	tree := fstest.MapFS{
		"changed.html": {Data: []byte("new content")},
		"keep.html":    {Data: []byte("same")},
		"new.html":     {Data: []byte("brand new")},
	}
	f := &fakeBackend{
		enumerable:     true,
		deleteOrphaned: true,
		have: core.State{
			"keep.html":          MD5Hex([]byte("same")),        // unchanged → skip
			"changed.html":       MD5Hex([]byte("old content")), // changed → upload
			"gone.html":          "x",                           // orphan → delete
			".well-known/p.json": "x",                           // orphan but protected → keep
		},
		protect: map[string]bool{".well-known/p.json": true},
	}
	res, err := Run(context.Background(), tree, f)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.puts, ","); got != "changed.html,new.html" {
		t.Errorf("uploads = %q, want changed.html,new.html", got)
	}
	if got := strings.Join(f.dels, ","); got != "gone.html" {
		t.Errorf("deletes = %q, want gone.html (protected path kept)", got)
	}
	if res.Total != 3 || res.Uploaded != 2 || res.Deleted != 1 {
		t.Errorf("result total=%d uploaded=%d deleted=%d, want 3/2/1", res.Total, res.Uploaded, res.Deleted)
	}
}

func TestRunTransactionalUploadsAllNoDeletes(t *testing.T) {
	// A non-enumerable (transactional) backend: every file uploads, nothing is deleted even
	// though the prior state lists an orphan — the platform handles removals via snapshots.
	tree := fstest.MapFS{"a.html": {Data: []byte("a")}, "b.html": {Data: []byte("b")}}
	f := &fakeBackend{enumerable: false, deleteOrphaned: true, have: core.State{"old.html": "x"}}
	res, err := Run(context.Background(), tree, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.puts) != 2 || len(f.dels) != 0 {
		t.Errorf("transactional: puts=%v dels=%v, want 2 puts and no deletes", f.puts, f.dels)
	}
	if res.Total != 2 {
		t.Errorf("total=%d, want 2", res.Total)
	}
}
