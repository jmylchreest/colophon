package build

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSweep(t *testing.T) {
	root := t.TempDir()
	write := func(rel string) string {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	keepA := write("index.html")
	keepB := write("posts/keep/index.html")
	write("posts/stale/index.html") // orphan file
	write("old-section/x.html")     // orphan whose dir should vanish too

	if err := sweep(root, map[string]struct{}{keepA: {}, keepB: {}}); err != nil {
		t.Fatal(err)
	}

	mustExist := func(rel string) {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s should have been kept: %v", rel, err)
		}
	}
	mustGone := func(rel string) {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("%s should have been removed", rel)
		}
	}
	mustExist("index.html")
	mustExist("posts/keep/index.html")
	mustGone("posts/stale/index.html")
	mustGone("posts/stale") // now-empty dir removed
	mustGone("old-section/x.html")
	mustGone("old-section") // now-empty dir removed
}
