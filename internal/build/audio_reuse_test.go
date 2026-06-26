package build

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/colophon/internal/generate"
)

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSidecarRenderMatches(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.wav")
	mustWrite(t, p, "x")
	mustWrite(t, p+".json", `{"provider":"minimax","model":"m1","voice":"v1"}`)
	if !sidecarRenderMatches(p, "minimax", "m1", "v1") {
		t.Error("exact render should match")
	}
	if sidecarRenderMatches(p, "elevenlabs", "m1", "v1") {
		t.Error("provider change should not match")
	}
	if sidecarRenderMatches(p, "minimax", "m1", "v2") {
		t.Error("voice change should not match")
	}
	// Missing sidecar, and a legacy sidecar without a provider, both reuse (no churn).
	if !sidecarRenderMatches(filepath.Join(dir, "absent.wav"), "x", "y", "z") {
		t.Error("missing sidecar should reuse")
	}
	mustWrite(t, p+".json", `{"voice":"v1","model":"m1"}`)
	if !sidecarRenderMatches(p, "minimax", "m1", "v1") {
		t.Error("pre-provider sidecar should reuse")
	}
}

func TestProduceReusePolicy(t *testing.T) {
	// reuse:exact (default) — a recorded renderer change re-renders the content file.
	ar, j := newTTSResolver(t, true)
	j.rs.provider = "minimax"
	mustWrite(t, j.cache, "OLD")
	mustWrite(t, j.cache+".json", `{"provider":"minimax","model":"m1","voice":"OTHER"}`)
	fake := &fakeSpeech{audio: []byte("\x01\x02\x03\x04")}
	withSpeechGen(j.rs, fake)
	out := ar.produce(j, time.Unix(0, 0))
	if fake.calls != 1 {
		t.Errorf("exact + renderer change: want re-render, calls=%d", fake.calls)
	}
	if len(out.bytes) < 4 || string(out.bytes[:4]) != "RIFF" {
		t.Error("want a fresh WAV")
	}

	// reuse:content — reuse regardless of the recorded renderer.
	ar2, j2 := newTTSResolver(t, true)
	j2.rs.reuseContent = true
	mustWrite(t, j2.cache, "CACHEDWAV")
	mustWrite(t, j2.cache+".json", `{"provider":"x","model":"y","voice":"z"}`)
	fake2 := &fakeSpeech{audio: []byte("x")}
	withSpeechGen(j2.rs, fake2)
	out2 := ar2.produce(j2, time.Unix(0, 0))
	if fake2.calls != 0 {
		t.Errorf("content: want reuse, calls=%d", fake2.calls)
	}
	if string(out2.bytes) != "CACHEDWAV" {
		t.Errorf("want cached bytes, got %q", out2.bytes)
	}
}

func TestProduceMigration(t *testing.T) {
	ar, j := newTTSResolver(t, true)
	j.rs.provider = "minimax"
	j.legacy = filepath.Join(j.rs.cacheDir, "legacy.wav") // pre-content-naming clip
	mustWrite(t, j.legacy, "LEGACYWAV")
	fake := &fakeSpeech{audio: []byte("x")}
	withSpeechGen(j.rs, fake)
	out := ar.produce(j, time.Unix(0, 0))
	if fake.calls != 0 {
		t.Errorf("migration: want adopt without re-render, calls=%d", fake.calls)
	}
	if string(out.bytes) != "LEGACYWAV" {
		t.Errorf("want legacy bytes, got %q", out.bytes)
	}
	if !fileExists(j.cache) {
		t.Error("adopted clip should be written under the content name")
	}
	if !sidecarRenderMatches(j.cache, "minimax", j.req.Model, j.req.Voice) {
		t.Error("adopted clip's sidecar should record the current renderer")
	}
}

func TestSpeechContentStem_Identity(t *testing.T) {
	a := generate.SpeechContentStem("post", "hello", nil)
	if a != generate.SpeechContentStem("post", "hello", nil) {
		t.Error("same text+label must be stable")
	}
	if a == generate.SpeechContentStem("post", "different", nil) {
		t.Error("different text must change the content stem")
	}
}
