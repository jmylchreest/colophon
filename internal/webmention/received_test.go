package webmention

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCacheRoundTrip(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cache")
	for _, key := range []string{"", "posts/example", "es/café"} {
		want := Mentions{Target: "/" + key, Mentions: []Mention{{Content: "Hello"}}}
		if err := SaveCached(dir, key, want); err != nil {
			t.Fatal(err)
		}
		got, err := LoadCached(dir, key)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("key %q: got %+v, %v", key, got, err)
		}
	}
	if _, err := LoadCached(filepath.Join(dir, "missing"), "post"); err != nil {
		t.Fatal(err)
	}
}

func TestCacheRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	for _, key := range []string{"..", "../outside", "posts/../../outside", "/absolute", ".", "a//b", `..\outside`, `C:\outside`, "bad\x00name"} {
		t.Run(key, func(t *testing.T) {
			if err := SaveCached(dir, key, Mentions{}); err == nil {
				t.Fatal("SaveCached accepted an unsafe key")
			}
			if _, err := LoadCached(dir, key); err == nil {
				t.Fatal("LoadCached accepted an unsafe key")
			}
		})
	}
}

func TestCacheRejectsEscapingSymlinks(t *testing.T) {
	dir, outside := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "post.json"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := SaveCached(dir, "escape/post", Mentions{}); err == nil {
		t.Fatal("wrote through an escaping symlink")
	}
	if _, err := LoadCached(dir, "escape/post"); err == nil {
		t.Fatal("read through an escaping symlink")
	}
	got, err := os.ReadFile(filepath.Join(outside, "post.json"))
	if err != nil || string(got) != "original" {
		t.Fatalf("outside file changed: %q, %v", got, err)
	}
}
