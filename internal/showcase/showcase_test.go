package showcase

import (
	"context"
	"io"
	"testing"
)

func TestDocument(t *testing.T) {
	c, src, err := Document()
	if err != nil {
		t.Fatal(err)
	}
	if c.Frontmatter.Title == "" {
		t.Error("showcase title missing from frontmatter")
	}
	if c.SourcePath != Slug+".md" {
		t.Errorf("SourcePath = %q, want %q", c.SourcePath, Slug+".md")
	}
	if len(c.Body) == 0 {
		t.Error("showcase body is empty")
	}

	// The embedded source serves the assets the showcase references.
	rc, err := src.Open(context.Background(), "assets/placeholder.png")
	if err != nil {
		t.Fatalf("open embedded asset: %v", err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if len(b) < 8 || string(b[1:4]) != "PNG" {
		t.Error("placeholder.png is not a PNG")
	}
	if _, ok := src.Resolve(context.Background(), "assets/sample.txt"); !ok {
		t.Error("a referenced asset should resolve")
	}
	if _, ok := src.Resolve(context.Background(), "assets/does-not-exist"); ok {
		t.Error("a missing asset must not resolve")
	}
}
