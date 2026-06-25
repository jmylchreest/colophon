package generate

import (
	"slices"
	"testing"

	"github.com/jmylchreest/colophon/internal/core"
)

func TestResolveSpeech_ElevenLabs(t *testing.T) {
	t.Setenv("ELEVENLABS_API_KEY", "sk-test")
	s, err := ResolveSpeech(core.SpeechGen{Provider: "elevenlabs"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Driver != driverElevenLabs {
		t.Errorf("driver = %q, want %q", s.Driver, driverElevenLabs)
	}
	if s.Model != "eleven_multilingual_v2" {
		t.Errorf("default model = %q", s.Model)
	}
	if s.APIKey != "sk-test" {
		t.Errorf("api key not read from env: %q", s.APIKey)
	}
	gen, err := NewSpeech(s)
	if err != nil {
		t.Fatalf("NewSpeech: %v", err)
	}
	if gen == nil {
		t.Fatal("nil generator")
	}
}

func TestResolveSpeech_ElevenLabs_AltEnv(t *testing.T) {
	t.Setenv("ELEVENLABS_API_KEY", "")
	t.Setenv("ELEVEN_API_KEY", "sk-alt")
	s, err := ResolveSpeech(core.SpeechGen{Provider: "elevenlabs"})
	if err != nil {
		t.Fatal(err)
	}
	if s.APIKey != "sk-alt" {
		t.Errorf("fallback env not read: %q", s.APIKey)
	}
}

func TestSpeechProviders_IncludesBoth(t *testing.T) {
	got := SpeechProviders()
	for _, want := range []string{"elevenlabs", "minimax"} {
		if !slices.Contains(got, want) {
			t.Errorf("SpeechProviders() = %v, missing %q", got, want)
		}
	}
}
