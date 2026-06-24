package build

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/colophon/internal/generate"
)

// fakeSpeech is a SpeechGenerator that returns canned audio and counts its calls — so a test can
// assert a generated reading costs exactly one render (no second pass for the waveform).
type fakeSpeech struct {
	audio []byte
	calls int
}

func (f *fakeSpeech) Generate(_ context.Context, _ generate.SpeechRequest) (generate.SpeechResult, error) {
	f.calls++
	return generate.SpeechResult{Bytes: f.audio, MIME: "audio/mpeg"}, nil
}

func newTTSResolver(t *testing.T, generateAI bool) (*audioResolver, *audioJob) {
	t.Helper()
	dir := t.TempDir()
	ar := &audioResolver{speech: &generate.SpeechSettings{}, generateAI: generateAI, cacheDir: dir}
	j := &audioJob{
		kind: "tts", outPath: genOutDir + "/clip.mp3", mime: "audio/mpeg",
		req:   generate.SpeechRequest{Text: "hello world", Voice: "v1", Format: "mp3", Model: "m1"},
		cache: filepath.Join(dir, "clip.mp3"),
	}
	return ar, j
}

// TestGenerateSingleRender: a generated reading makes exactly one provider call (the waveform is
// now derived client-side from the audio), and no peaks sidecar is produced server-side.
func TestGenerateSingleRender(t *testing.T) {
	ar, j := newTTSResolver(t, true)
	fake := &fakeSpeech{audio: []byte("MP3DATA")}
	ensure := func() (generate.SpeechGenerator, error) { return fake, nil }

	out := ar.produce(j, ensure, time.Unix(0, 0))
	if string(out.bytes) != "MP3DATA" {
		t.Fatalf("audio should publish; got %q", out.bytes)
	}
	if fake.calls != 1 {
		t.Errorf("a reading must cost exactly one render, got %d", fake.calls)
	}
	if len(j.peaks) != 0 {
		t.Errorf("no server-side peaks expected, got %d", len(j.peaks))
	}
	// The cache sidecar carries metadata but no peaks.
	b, err := os.ReadFile(j.cache + ".json")
	if err != nil {
		t.Fatalf("cache sidecar missing: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["peaks"]; ok {
		t.Error("cache sidecar should no longer store peaks")
	}
}

// TestCacheHitReadsLegacyPeaks: a clip cached by an older build whose sidecar carries peaks still
// surfaces them (read-compat), with no provider call.
func TestCacheHitReadsLegacyPeaks(t *testing.T) {
	ar, j := newTTSResolver(t, true)
	if err := os.WriteFile(j.cache, []byte("CACHEDMP3"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(j.cache+".json", []byte(`{"voice":"v1","peaks":[0.1,0.9,0.4]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := &fakeSpeech{audio: []byte("x")}
	ensure := func() (generate.SpeechGenerator, error) { return fake, nil }

	out := ar.produce(j, ensure, time.Unix(0, 0))
	if string(out.bytes) != "CACHEDMP3" {
		t.Fatalf("must reuse cached audio; got %q", out.bytes)
	}
	if fake.calls != 0 {
		t.Errorf("cache hit must not call the provider, got %d", fake.calls)
	}
	if len(j.peaks) != 3 {
		t.Errorf("legacy peaks should be read for compat, got %d", len(j.peaks))
	}
}
