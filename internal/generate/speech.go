package generate

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jmylchreest/colophon/internal/core"
)

// SpeechRequest is a single text-to-speech request.
type SpeechRequest struct {
	Text   string
	Voice  string
	Format string
	Model  string
}

// SpeechResult is generated audio plus its MIME type.
type SpeechResult struct {
	Bytes []byte
	MIME  string
}

// SpeechGenerator turns text into spoken audio. Implementations are provider drivers
// constructed via NewSpeech.
type SpeechGenerator interface {
	Generate(ctx context.Context, req SpeechRequest) (SpeechResult, error)
}

// SpeechSettings is a fully-resolved speech configuration (profile defaults + overrides).
type SpeechSettings struct {
	Provider    string
	Driver      string
	Model       string
	Voice       string
	Format      string
	OutputDir   string
	BaseURL     string
	APIPath     string
	APIKey      string
	Concurrency int
	Transcript  core.SpeechTranscript
	Retry       RetryPolicy // rate-limit backoff; zero value = fail fast
}

type speechProfile struct {
	driver       string
	baseURL      string
	apiPath      string
	defaultModel string
	defaultVoice string
	keyEnv       []string
}

// speechProfiles are the built-in TTS provider presets. MiniMax is implemented; others
// can be added by writing a driver and an entry here.
var speechProfiles = map[string]speechProfile{
	driverMiniMax: {driver: driverMiniMax, baseURL: "https://api.minimax.io/v1", apiPath: "/t2a_v2", defaultModel: "speech-2.6-hd", defaultVoice: "English_Graceful_Lady", keyEnv: []string{"MINIMAX_API_KEY"}},
}

// SpeechProviders lists the configurable speech provider names.
func SpeechProviders() []string { return []string{driverMiniMax} }

// ResolveSpeech applies the provider profile to a speech config block, layering explicit
// fields over the defaults and reading the API key from the environment when not inline.
func ResolveSpeech(g core.SpeechGen) (SpeechSettings, error) {
	name := strings.ToLower(strings.TrimSpace(g.Provider))
	if name == "" {
		return SpeechSettings{}, fmt.Errorf("no provider configured")
	}
	p, ok := speechProfiles[name]
	if !ok {
		return SpeechSettings{}, fmt.Errorf("unknown speech provider %q (have: %s)", name, strings.Join(SpeechProviders(), ", "))
	}
	s := SpeechSettings{
		Provider:    name,
		Driver:      p.driver,
		Model:       firstNonEmpty(g.Model, p.defaultModel),
		Voice:       firstNonEmpty(g.Voice, p.defaultVoice),
		Format:      strings.ToLower(firstNonEmpty(g.Format, "mp3")),
		OutputDir:   firstNonEmpty(g.OutputDir, DefaultOutputDir),
		BaseURL:     firstNonEmpty(g.BaseURL, p.baseURL),
		APIPath:     firstNonEmpty(g.APIPath, p.apiPath),
		APIKey:      strings.TrimSpace(g.APIKey),
		Concurrency: g.Concurrency,
		Transcript:  g.Transcript,
	}
	if s.Concurrency <= 0 {
		s.Concurrency = DefaultConcurrency
	}
	if s.APIKey == "" {
		for _, k := range p.keyEnv {
			if v := strings.TrimSpace(os.Getenv(k)); v != "" {
				s.APIKey = v
				break
			}
		}
	}
	return s, nil
}

// NewSpeech constructs the speech generator for these settings.
func NewSpeech(s SpeechSettings) (SpeechGenerator, error) {
	if s.APIKey == "" {
		return nil, fmt.Errorf("provider %q: no API key (set api_key or the provider's env var)", s.Provider)
	}
	switch s.Driver {
	case driverMiniMax:
		return withSpeechRetry(&minimaxSpeech{endpoint: s.BaseURL + s.APIPath, apiKey: s.APIKey}, s.Retry), nil
	default:
		return nil, fmt.Errorf("unknown speech driver %q", s.Driver)
	}
}

// SpeechStem is the deterministic, extension-less cache name for a speech request. The
// label (e.g. the post slug) is a readable prefix; the hash covers everything affecting
// the audio (provider, model, voice, format, text).
func SpeechStem(provider, model, voice, format, label, text string) string {
	key := CacheKey(provider, model, text, voice, map[string]string{"format": format})
	return promptSlug(label) + "-" + key
}

// AudioMIME maps an audio format to its MIME type (for enclosures and players).
func AudioMIME(format string) string {
	switch strings.ToLower(format) {
	case "mp3":
		return "audio/mpeg"
	case "wav", "pcmu_wav":
		return "audio/wav"
	case "flac":
		return "audio/flac"
	case "opus":
		return "audio/opus"
	case "pcm", "pcmu_raw":
		return "audio/L16"
	default:
		return "application/octet-stream"
	}
}
