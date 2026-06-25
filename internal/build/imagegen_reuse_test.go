package build

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/colophon/internal/generate"
)

type fakeImageGen struct {
	calls int
	bytes []byte
}

func (f *fakeImageGen) Generate(_ context.Context, _ generate.ImageRequest) (generate.ImageResult, error) {
	f.calls++
	return generate.ImageResult{Bytes: f.bytes, MIME: "image/png"}, nil
}

var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...)

func TestImageSidecarRenderMatches(t *testing.T) {
	dir := t.TempDir()
	gr := &genResolver{s: &generate.Settings{Provider: "google", Model: "m"}, cacheDir: dir}
	mustWrite(t, filepath.Join(dir, "img.png"), "x")
	mustWrite(t, filepath.Join(dir, "img.png.json"), `{"provider":"google","model":"m"}`)
	if !gr.sidecarRenderMatches("img.png") {
		t.Error("same provider/model should match")
	}
	mustWrite(t, filepath.Join(dir, "img.png.json"), `{"provider":"openai","model":"m"}`)
	if gr.sidecarRenderMatches("img.png") {
		t.Error("provider change should not match")
	}
	mustWrite(t, filepath.Join(dir, "img.png.json"), `{"prompt":"p"}`) // pre-provider sidecar
	if !gr.sidecarRenderMatches("img.png") {
		t.Error("sidecar without provider should reuse")
	}
}

func TestImageProduceReusePolicy(t *testing.T) {
	now := time.Unix(0, 0)
	// reuse:exact — provider change re-generates.
	dir := t.TempDir()
	gr := &genResolver{s: &generate.Settings{Provider: "google", Model: "m"}, cacheDir: dir}
	j := &genJob{stem: "img-c", legacyStem: "img-l", req: generate.ImageRequest{Prompt: "p", Model: "m"}}
	mustWrite(t, filepath.Join(dir, "img-c.png"), "OLD")
	mustWrite(t, filepath.Join(dir, "img-c.png.json"), `{"provider":"openai","model":"m"}`)
	fake := &fakeImageGen{bytes: pngBytes}
	gr.produce(j, true, func() (generate.ImageGenerator, error) { return fake, nil }, now)
	if fake.calls != 1 {
		t.Errorf("exact + provider change: want regenerate, calls=%d", fake.calls)
	}
	if j.filename != "img-c.png" {
		t.Errorf("regenerated file should be content-named, got %q", j.filename)
	}

	// reuse:content — reuse regardless of recorded renderer.
	dir2 := t.TempDir()
	gr2 := &genResolver{s: &generate.Settings{Provider: "google", Model: "m"}, reuseContent: true, cacheDir: dir2}
	j2 := &genJob{stem: "img-c", legacyStem: "img-l", req: generate.ImageRequest{Prompt: "p", Model: "m"}}
	mustWrite(t, filepath.Join(dir2, "img-c.png"), "CACHED")
	mustWrite(t, filepath.Join(dir2, "img-c.png.json"), `{"provider":"openai","model":"m"}`)
	fake2 := &fakeImageGen{bytes: pngBytes}
	gr2.produce(j2, true, func() (generate.ImageGenerator, error) { return fake2, nil }, now)
	if fake2.calls != 0 {
		t.Errorf("content: want reuse, calls=%d", fake2.calls)
	}
}

func TestImageProduceMigration(t *testing.T) {
	dir := t.TempDir()
	gr := &genResolver{s: &generate.Settings{Provider: "google", Model: "m"}, cacheDir: dir}
	j := &genJob{stem: "img-c", legacyStem: "img-l", req: generate.ImageRequest{Prompt: "p", Model: "m"}}
	mustWrite(t, filepath.Join(dir, "img-l.jpg"), "LEGACY")
	mustWrite(t, filepath.Join(dir, "img-l.jpg.json"), `{"provider":"google","model":"m"}`)
	fake := &fakeImageGen{bytes: pngBytes}
	gr.produce(j, true, func() (generate.ImageGenerator, error) { return fake, nil }, time.Unix(0, 0))
	if fake.calls != 0 {
		t.Errorf("migration: want adopt without regenerate, calls=%d", fake.calls)
	}
	if j.filename != "img-c.jpg" {
		t.Errorf("adopted file should keep ext under content name, got %q", j.filename)
	}
	if !fileExists(filepath.Join(dir, "img-c.jpg.json")) {
		t.Error("adopted image's sidecar should be copied")
	}
}
