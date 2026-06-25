package generate

import (
	"fmt"
	"os"
	"strings"

	"github.com/jmylchreest/colophon/internal/core"
)

// DefaultOutputDir is where generated images and their sidecars are cached when the
// config sets no output_dir. It lives under content/ so the cache ships with the
// site through the normal asset pipeline (and is committed, making builds reproducible).
const DefaultOutputDir = "content/assets/generated"

const (
	driverGoogle     = "google"
	driverOpenAI     = "openai"
	driverMiniMax    = "minimax"
	driverElevenLabs = "elevenlabs"
)

// DefaultConcurrency is the max number of images generated in parallel when the
// config does not set generation.image.concurrency.
const DefaultConcurrency = 5

// Settings is a fully-resolved image-generation configuration: a provider profile's
// defaults with the user's explicit overrides layered on top. It carries everything
// needed to compute cache paths; constructing a live generator (New) additionally
// requires APIKey to be non-empty.
type Settings struct {
	Provider      string
	Driver        string
	Model         string
	OutputDir     string
	BaseURL       string
	APIPath       string
	APIKey        string
	Defaults      map[string]string
	Concurrency   int
	TrimLetterbox bool
	Retry         RetryPolicy // rate-limit backoff; zero value = fail fast
}

type profile struct {
	driver       string
	baseURL      string
	apiPath      string
	defaultModel string
	keyEnv       []string
}

// profiles are the built-in provider presets. `custom` carries no endpoint: the
// user supplies base_url/api_path (and a key) for any other OpenAI-compatible host.
var profiles = map[string]profile{
	driverGoogle:  {driver: driverGoogle, baseURL: "https://generativelanguage.googleapis.com", defaultModel: "gemini-3.1-flash-image", keyEnv: []string{"GEMINI_API_KEY", "GOOGLE_API_KEY"}},
	driverMiniMax: {driver: driverMiniMax, baseURL: "https://api.minimax.io/v1", apiPath: "/image_generation", defaultModel: "image-01", keyEnv: []string{"MINIMAX_API_KEY"}},
	"openai":      {driver: driverOpenAI, baseURL: "https://api.openai.com/v1", apiPath: "/images/generations", defaultModel: "gpt-image-1", keyEnv: []string{"OPENAI_API_KEY"}},
	"together":    {driver: driverOpenAI, baseURL: "https://api.together.ai/v1", apiPath: "/images/generations", keyEnv: []string{"TOGETHER_API_KEY"}},
	"deepinfra":   {driver: driverOpenAI, baseURL: "https://api.deepinfra.com/v1/openai", apiPath: "/images/generations", keyEnv: []string{"DEEPINFRA_API_KEY"}},
	"custom":      {driver: driverOpenAI},
}

// Providers lists the configurable provider names, for diagnostics/help.
func Providers() []string {
	return []string{driverGoogle, driverMiniMax, "openai", "together", "deepinfra", "custom"}
}

// Resolve applies the provider profile to a config block, layering explicit fields
// over the profile defaults and reading the API key from the environment when not
// given inline. An unknown or unset provider is an error.
func Resolve(g core.ImageGen) (Settings, error) {
	name := strings.ToLower(strings.TrimSpace(g.Provider))
	if name == "" {
		return Settings{}, fmt.Errorf("no provider configured")
	}
	p, ok := profiles[name]
	if !ok {
		return Settings{}, fmt.Errorf("unknown provider %q (have: %s)", name, strings.Join(Providers(), ", "))
	}
	s := Settings{
		Provider:      name,
		Driver:        p.driver,
		Model:         firstNonEmpty(g.Model, p.defaultModel),
		OutputDir:     firstNonEmpty(g.OutputDir, DefaultOutputDir),
		BaseURL:       firstNonEmpty(g.BaseURL, p.baseURL),
		APIPath:       firstNonEmpty(g.APIPath, p.apiPath),
		APIKey:        strings.TrimSpace(g.APIKey),
		Defaults:      g.Defaults,
		Concurrency:   g.Concurrency,
		TrimLetterbox: g.Postprocess.TrimsLetterbox(),
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
	if s.Model == "" {
		return Settings{}, fmt.Errorf("provider %q: no model set (profile has no default; set generation.image.model)", name)
	}
	return s, nil
}

// ImageStem is the deterministic, extension-less cache name for a prompt (with its
// resolved system prompt) + params under these settings. The build calls it to
// identify a `gen:` ref's image without contacting the provider, so the stem it
// computes always matches what Request would generate.
func (s Settings) ImageStem(prompt, system string, params map[string]string) string {
	return Stem(s.Provider, s.Model, prompt, system, s.mergeParams(params))
}

// Request builds the generation request for a prompt (with its resolved system prompt)
// + params, applying the same param merge as ImageStem so the produced bytes match the
// resolved cache name.
func (s Settings) Request(prompt, system string, params map[string]string) ImageRequest {
	return ImageRequest{Prompt: prompt, System: system, Model: s.Model, Params: s.mergeParams(params)}
}

// mergeParams overlays a request's params over the settings' defaults, so a per-ref
// ?aspect=… wins over the configured default but inherits anything it doesn't set.
func (s Settings) mergeParams(p map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range s.Defaults {
		out[k] = v
	}
	for k, v := range p {
		out[k] = v
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
