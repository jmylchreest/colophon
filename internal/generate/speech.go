package generate

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/jmylchreest/colophon/internal/core"
)

// SpeechRequest is a single text-to-speech request.
type SpeechRequest struct {
	Text          string
	Voice         string
	Model         string
	Pronunciation []Pronunciation // provider-agnostic overrides; nil when none apply
}

// SpeechResult is generated audio. The bytes are raw mono 16-bit little-endian PCM at
// SampleRate Hz; the caller wraps them in a container (WAV). MIME describes the raw bytes.
type SpeechResult struct {
	Bytes      []byte
	MIME       string // raw sample format, e.g. "audio/L16"
	SampleRate int    // PCM sample rate in Hz
}

// SpeechGenerator turns text into spoken audio. Implementations are provider drivers
// constructed via NewSpeech.
type SpeechGenerator interface {
	Generate(ctx context.Context, req SpeechRequest) (SpeechResult, error)
}

// SpeechSettings is a fully-resolved speech configuration (profile defaults + overrides).
type SpeechSettings struct {
	Provider          string
	Driver            string
	Model             string
	Voice             string
	OutputDir         string
	PronunciationDict core.PronunciationDicts // per-language pronunciation dictionary refs ("" key = site default language)
	BaseURL           string
	APIPath           string
	APIKey            string
	Concurrency       int
	SiteID            string // stable per-site identifier (e.g. host), namespaces shared-account state
	Reuse             string // "exact" (default) | "content" — cross-render cache reuse policy
	Transcript        core.SpeechTranscript
	Retry             RetryPolicy        // rate-limit backoff; zero value = fail fast
	elevenLabsLocator *elevenLabsLocator // set by PrepareSpeech; ElevenLabs IPA dictionary version
}

type speechProfile struct {
	driver       string
	baseURL      string
	apiPath      string
	defaultModel string
	defaultVoice string
	keyEnv       []string
}

// speechProfiles are the built-in TTS provider presets. New providers are added by writing a
// driver (see minimax_speech.go / elevenlabs_speech.go) and an entry here.
var speechProfiles = map[string]speechProfile{
	driverMiniMax:    {driver: driverMiniMax, baseURL: minimaxBaseURL, apiPath: minimaxAPIPath, defaultModel: minimaxDefaultModel, defaultVoice: minimaxDefaultVoice, keyEnv: []string{"MINIMAX_API_KEY"}},
	driverElevenLabs: {driver: driverElevenLabs, baseURL: elevenLabsBaseURL, apiPath: elevenLabsAPIPath, defaultModel: elevenLabsDefaultModel, defaultVoice: elevenLabsDefaultVoice, keyEnv: []string{"ELEVENLABS_API_KEY", "COLOPHON_ELEVENLABS_API_KEY", "ELEVEN_API_KEY"}},
}

// SpeechProviders lists the configurable speech provider names (sorted for stable messages).
func SpeechProviders() []string {
	names := make([]string, 0, len(speechProfiles))
	for n := range speechProfiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

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
		Provider:          name,
		Driver:            p.driver,
		Model:             firstNonEmpty(g.Model, p.defaultModel),
		Voice:             firstNonEmpty(g.Voice, p.defaultVoice),
		OutputDir:         firstNonEmpty(g.OutputDir, DefaultOutputDir),
		PronunciationDict: g.PronunciationDict,
		Reuse:             strings.ToLower(strings.TrimSpace(g.Reuse)),
		BaseURL:           firstNonEmpty(g.BaseURL, p.baseURL),
		APIPath:           firstNonEmpty(g.APIPath, p.apiPath),
		APIKey:            strings.TrimSpace(g.APIKey),
		Concurrency:       g.Concurrency,
		Transcript:        g.Transcript,
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
	case driverElevenLabs:
		return withSpeechRetry(&elevenLabsSpeech{endpoint: s.BaseURL + s.APIPath, apiKey: s.APIKey, locator: s.elevenLabsLocator}, s.Retry), nil
	default:
		return nil, fmt.Errorf("unknown speech driver %q", s.Driver)
	}
}

// SpeechContentStem names a reading by WHAT is said — the text plus the applied pronunciation.
// This is the published, renderer-independent identity, so the audio URL is stable when the
// provider/model/voice change and only moves when the text itself changes.
func SpeechContentStem(label, text string, pronunciation []Pronunciation) string {
	var extra map[string]string
	if len(pronunciation) > 0 {
		extra = map[string]string{"pron": pronunciationKey(pronunciation)}
	}
	return promptSlug(label) + "-" + CacheKey("", "", text, "", extra)
}

// SpeechStem is the pre-split single-hash cache name (content+render in one key). Retained so a
// build can find and adopt audio cached by an older colophon during the move to content naming.
func SpeechStem(provider, model, voice, label, text string, pronunciation []Pronunciation) string {
	var extra map[string]string
	if len(pronunciation) > 0 {
		extra = map[string]string{"pron": pronunciationKey(pronunciation)}
	}
	return promptSlug(label) + "-" + CacheKey(provider, model, text, voice, extra)
}
