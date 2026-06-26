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
	rc, err := src.Open(context.Background(), "assets/sample-image.jpg")
	if err != nil {
		t.Fatalf("open embedded asset: %v", err)
	}
	b, _ := io.ReadAll(rc)
	_ = rc.Close()
	if len(b) < 3 || b[0] != 0xFF || b[1] != 0xD8 {
		t.Error("sample-image.jpg is not a JPEG")
	}
	for _, a := range []string{"assets/sample.txt", "assets/reading.wav", "assets/sample-video.mp4"} {
		if _, ok := src.Resolve(context.Background(), a); !ok {
			t.Errorf("referenced asset %s should resolve", a)
		}
	}
	// Glossary terms are provided for --showcase decoration.
	if Glossary()["TTS"].Definition == "" {
		t.Error("showcase glossary should define TTS")
	}
	if _, ok := src.Resolve(context.Background(), "assets/does-not-exist"); ok {
		t.Error("a missing asset must not resolve")
	}
}
