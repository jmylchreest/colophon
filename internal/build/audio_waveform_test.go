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

// fakeSpeech is a SpeechGenerator that returns canned PCM and counts its calls — so a test can
// assert a generated reading costs exactly one render (no second pass for the waveform).
type fakeSpeech struct {
	audio []byte
	calls int
}

func (f *fakeSpeech) Generate(_ context.Context, _ generate.SpeechRequest) (generate.SpeechResult, error) {
	f.calls++
	return generate.SpeechResult{Bytes: f.audio, MIME: "audio/L16", SampleRate: 16000}, nil
}

func newTTSResolver(t *testing.T, generateAI bool) (*audioResolver, *audioJob) {
	t.Helper()
	dir := t.TempDir()
	rs := &resolvedSpeech{settings: generate.SpeechSettings{Model: "m1"}, cacheDir: dir, gens: map[string]*dictGen{}}
	ar := &audioResolver{generateAI: generateAI, profiles: map[string]*resolvedSpeech{"default": rs}}
	j := &audioJob{
		kind: "tts", outPath: genOutDir + "/clip" + ttsOutputExt, mime: ttsOutputMIME,
		req:   generate.SpeechRequest{Text: "hello world", Voice: "v1", Model: "m1"},
		cache: filepath.Join(dir, "clip"+ttsOutputExt),
		rs:    rs,
	}
	return ar, j
}

// withSpeechGen pre-injects a generator into a resolved profile (under the no-dict slot),
// consuming its once-cell so produce() uses the fake instead of building a real generator.
func withSpeechGen(rs *resolvedSpeech, g generate.SpeechGenerator) {
	dg := &dictGen{gen: g}
	dg.once.Do(func() {})
	rs.gens[""] = dg
}

// TestGenerateSingleRender: a generated reading makes exactly one provider call — the waveform
// peaks are derived from those same samples, not a second render — the published bytes are
// WAV-wrapped, and the peaks are stored in the cache sidecar for reuse.
func TestGenerateSingleRender(t *testing.T) {
	ar, j := newTTSResolver(t, true)
	// Enough PCM (>= waveformBuckets samples) with varying amplitude that peaks can be derived.
	pcm := make([]byte, 0, 512)
	for i := 0; i < 256; i++ {
		v := int16((i%32 - 16) * 200)
		pcm = append(pcm, byte(v), byte(v>>8))
	}
	fake := &fakeSpeech{audio: pcm}
	withSpeechGen(j.rs, fake)

	out := ar.produce(j, time.Unix(0, 0))
	if len(out.bytes) < 44 || string(out.bytes[:4]) != "RIFF" {
		t.Fatalf("audio should publish as WAV; got %d bytes %q", len(out.bytes), out.bytes[:min(4, len(out.bytes))])
	}
	if fake.calls != 1 {
		t.Errorf("a reading must cost exactly one render, got %d", fake.calls)
	}
	if len(j.peaks) == 0 {
		t.Error("server-side peaks should be derived from the rendered samples")
	}
	// The cache sidecar stores the peaks, so a later cache hit reuses them without re-analysing.
	b, err := os.ReadFile(j.cache + ".json")
	if err != nil {
		t.Fatalf("cache sidecar missing: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if pk, ok := m["peaks"].([]any); !ok || len(pk) == 0 {
		t.Error("cache sidecar should store the precomputed peaks")
	}
}

// TestCacheHitReadsLegacyPeaks: a clip cached by an older build whose sidecar carries peaks still
// surfaces them (read-compat), with no provider call.
func TestCacheHitReadsLegacyPeaks(t *testing.T) {
	ar, j := newTTSResolver(t, true)
	if err := os.WriteFile(j.cache, []byte("CACHEDWAV"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(j.cache+".json", []byte(`{"voice":"v1","peaks":[0.1,0.9,0.4]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := &fakeSpeech{audio: []byte("x")}
	withSpeechGen(j.rs, fake)

	out := ar.produce(j, time.Unix(0, 0))
	if string(out.bytes) != "CACHEDWAV" {
		t.Fatalf("must reuse cached audio; got %q", out.bytes)
	}
	if fake.calls != 0 {
		t.Errorf("cache hit must not call the provider, got %d", fake.calls)
	}
	if len(j.peaks) != 3 {
		t.Errorf("legacy peaks should be read for compat, got %d", len(j.peaks))
	}
}
