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

// imgJob builds a genJob whose resolved profile renders with the given provider/model into dir.
func imgJob(dir, provider string, reuseContent bool) *genJob {
	return &genJob{
		stem: "img-c", legacyStem: "img-l", req: generate.ImageRequest{Prompt: "p", Model: "m"},
		ri: &resolvedImage{settings: generate.Settings{Provider: provider, Model: "m"}, cacheDir: dir, reuseContent: reuseContent},
	}
}

// withImageGen pre-injects a generator into a resolved profile, consuming its once-cell so
// produce() uses the fake instead of building a real generator.
func withImageGen(ri *resolvedImage, g generate.ImageGenerator) {
	ri.gen = g
	ri.once.Do(func() {})
}

func TestImageSidecarRenderMatches(t *testing.T) {
	dir := t.TempDir()
	gr := &genResolver{}
	j := imgJob(dir, "google", false)
	mustWrite(t, filepath.Join(dir, "img.png"), "x")
	mustWrite(t, filepath.Join(dir, "img.png.json"), `{"provider":"google","model":"m"}`)
	if !gr.sidecarRenderMatches(j, "img.png") {
		t.Error("same provider/model should match")
	}
	mustWrite(t, filepath.Join(dir, "img.png.json"), `{"provider":"openai","model":"m"}`)
	if gr.sidecarRenderMatches(j, "img.png") {
		t.Error("provider change should not match")
	}
	mustWrite(t, filepath.Join(dir, "img.png.json"), `{"prompt":"p"}`) // pre-provider sidecar
	if !gr.sidecarRenderMatches(j, "img.png") {
		t.Error("sidecar without provider should reuse")
	}
}

func TestImageProduceReusePolicy(t *testing.T) {
	now := time.Unix(0, 0)
	gr := &genResolver{}
	// reuse:exact — provider change re-generates.
	dir := t.TempDir()
	j := imgJob(dir, "google", false)
	mustWrite(t, filepath.Join(dir, "img-c.png"), "OLD")
	mustWrite(t, filepath.Join(dir, "img-c.png.json"), `{"provider":"openai","model":"m"}`)
	fake := &fakeImageGen{bytes: pngBytes}
	withImageGen(j.ri, fake)
	gr.produce(j, true, now)
	if fake.calls != 1 {
		t.Errorf("exact + provider change: want regenerate, calls=%d", fake.calls)
	}
	if j.filename != "img-c.png" {
		t.Errorf("regenerated file should be content-named, got %q", j.filename)
	}

	// reuse:content — reuse regardless of recorded renderer.
	dir2 := t.TempDir()
	j2 := imgJob(dir2, "google", true)
	mustWrite(t, filepath.Join(dir2, "img-c.png"), "CACHED")
	mustWrite(t, filepath.Join(dir2, "img-c.png.json"), `{"provider":"openai","model":"m"}`)
	fake2 := &fakeImageGen{bytes: pngBytes}
	withImageGen(j2.ri, fake2)
	gr.produce(j2, true, now)
	if fake2.calls != 0 {
		t.Errorf("content: want reuse, calls=%d", fake2.calls)
	}
}

func TestImageProduceMigration(t *testing.T) {
	dir := t.TempDir()
	gr := &genResolver{}
	j := imgJob(dir, "google", false)
	mustWrite(t, filepath.Join(dir, "img-l.jpg"), "LEGACY")
	mustWrite(t, filepath.Join(dir, "img-l.jpg.json"), `{"provider":"google","model":"m"}`)
	fake := &fakeImageGen{bytes: pngBytes}
	withImageGen(j.ri, fake)
	gr.produce(j, true, time.Unix(0, 0))
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
