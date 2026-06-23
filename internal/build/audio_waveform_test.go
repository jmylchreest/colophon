package build

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmylchreest/colophon/internal/generate"
)

// fakeSpeech is a SpeechGenerator that returns canned audio: a distinct payload for the mp3
// render and the pcm (waveform) render, with an optional error on the pcm pass.
type fakeSpeech struct {
	mp3    []byte
	pcm    []byte
	pcmErr error
	calls  int
}

func (f *fakeSpeech) Generate(_ context.Context, req generate.SpeechRequest) (generate.SpeechResult, error) {
	f.calls++
	if req.Format == "pcm" {
		if f.pcmErr != nil {
			return generate.SpeechResult{}, f.pcmErr
		}
		return generate.SpeechResult{Bytes: f.pcm, MIME: "audio/pcm"}, nil
	}
	return generate.SpeechResult{Bytes: f.mp3, MIME: "audio/mpeg"}, nil
}

// pcmRamp is a valid little-endian 16-bit mono PCM buffer with enough varied, non-zero samples
// that peaksFromPCM accepts it.
func pcmRamp() []byte {
	b := make([]byte, 0, 512)
	for i := 0; i < 256; i++ {
		v := int16((i%50 + 1) * 400)
		b = append(b, byte(v), byte(uint16(v)>>8))
	}
	return b
}

func newTTSResolver(t *testing.T, generateAI bool) (*audioResolver, *audioJob) {
	t.Helper()
	dir := t.TempDir()
	ar := &audioResolver{
		speech:     &generate.SpeechSettings{Waveform: true},
		generateAI: generateAI,
		cacheDir:   dir,
	}
	j := &audioJob{
		kind: "tts", outPath: genOutDir + "/clip.mp3", mime: "audio/mpeg",
		req:   generate.SpeechRequest{Text: "hello world", Voice: "v1", Format: "mp3", Model: "m1"},
		cache: filepath.Join(dir, "clip.mp3"),
	}
	return ar, j
}

// TestWaveformGenerateWarnsOnPCMFailure: when the mp3 generates but the pcm/waveform render
// errors, the audio still publishes and the failure surfaces as a warning (not swallowed).
func TestWaveformGenerateWarnsOnPCMFailure(t *testing.T) {
	ar, j := newTTSResolver(t, true)
	fake := &fakeSpeech{mp3: []byte("MP3DATA"), pcmErr: errors.New("format pcm not supported")}
	ensure := func() (generate.SpeechGenerator, error) { return fake, nil }

	out := ar.produce(j, ensure, time.Unix(0, 0))
	if string(out.bytes) != "MP3DATA" {
		t.Fatalf("audio should still publish; got %q", out.bytes)
	}
	if len(j.peaks) != 0 {
		t.Errorf("no peaks expected when pcm fails, got %d", len(j.peaks))
	}
	if len(out.warns) == 0 {
		t.Fatal("pcm failure must produce a warning, not be swallowed")
	}
	if !strings.Contains(strings.Join(out.warns, " "), "format pcm not supported") {
		t.Errorf("warning should carry the provider error; got %v", out.warns)
	}
}

// TestWaveformGenerateSucceeds: a working pcm render yields peaks, persisted in the cache sidecar.
func TestWaveformGenerateSucceeds(t *testing.T) {
	ar, j := newTTSResolver(t, true)
	fake := &fakeSpeech{mp3: []byte("MP3DATA"), pcm: pcmRamp()}
	ensure := func() (generate.SpeechGenerator, error) { return fake, nil }

	out := ar.produce(j, ensure, time.Unix(0, 0))
	if len(out.warns) != 0 {
		t.Errorf("no warnings expected, got %v", out.warns)
	}
	if len(j.peaks) != waveformBuckets {
		t.Fatalf("want %d peaks, got %d", waveformBuckets, len(j.peaks))
	}
	if peaks := sidecarPeaks(t, j.cache+".json"); len(peaks) != waveformBuckets {
		t.Errorf("cache sidecar should persist %d peaks, got %d", waveformBuckets, len(peaks))
	}
}

// TestWaveformBackfillOnCacheHit: a cached clip whose sidecar lacks peaks gets just its waveform
// (a pcm-only render) without re-synthesizing the audio, and the sidecar is updated in place,
// preserving its other fields.
func TestWaveformBackfillOnCacheHit(t *testing.T) {
	ar, j := newTTSResolver(t, true)
	if err := os.WriteFile(j.cache, []byte("CACHEDMP3"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A pre-existing sidecar with no peaks (as an older colophon would have written).
	if err := os.WriteFile(j.cache+".json", []byte(`{"voice":"v1","model":"m1","generated":"2026-01-01T00:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := &fakeSpeech{mp3: []byte("SHOULD-NOT-BE-USED"), pcm: pcmRamp()}
	ensure := func() (generate.SpeechGenerator, error) { return fake, nil }

	out := ar.produce(j, ensure, time.Unix(0, 0))
	if string(out.bytes) != "CACHEDMP3" {
		t.Fatalf("must reuse cached audio, not regenerate; got %q", out.bytes)
	}
	if fake.calls != 1 {
		t.Errorf("backfill should make exactly one (pcm) call, not regenerate the mp3; got %d", fake.calls)
	}
	if len(j.peaks) != waveformBuckets {
		t.Fatalf("backfill should compute %d peaks, got %d", waveformBuckets, len(j.peaks))
	}
	// Sidecar updated with peaks, other fields preserved.
	var m map[string]any
	b, _ := os.ReadFile(j.cache + ".json")
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["peaks"]; !ok {
		t.Error("sidecar should now carry peaks")
	}
	if m["generated"] != "2026-01-01T00:00:00Z" {
		t.Errorf("backfill should preserve the original generated time; got %v", m["generated"])
	}
}

// TestWaveformNoBackfillWithoutGenerateAI: a plain build (no --generate-ai) never makes a
// network call to backfill a missing waveform.
func TestWaveformNoBackfillWithoutGenerateAI(t *testing.T) {
	ar, j := newTTSResolver(t, false)
	if err := os.WriteFile(j.cache, []byte("CACHEDMP3"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(j.cache+".json", []byte(`{"voice":"v1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	fake := &fakeSpeech{mp3: []byte("x"), pcm: pcmRamp()}
	ensure := func() (generate.SpeechGenerator, error) { return fake, nil }

	out := ar.produce(j, ensure, time.Unix(0, 0))
	if fake.calls != 0 {
		t.Errorf("plain build must not call the provider; got %d calls", fake.calls)
	}
	if len(j.peaks) != 0 || len(out.warns) != 0 {
		t.Errorf("plain build should be inert; peaks=%d warns=%v", len(j.peaks), out.warns)
	}
}

func sidecarPeaks(t *testing.T, path string) []float64 {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return parsePeaks(b)
}
