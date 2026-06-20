package core

import "strings"

// Generation configures colophon's optional AI media generation, one block per modality
// (image, speech). Each block names a provider profile; credentials are shared across
// modalities through the per-provider env var, or set per block via api_key.
type Generation struct {
	// Enabled is the master switch for all AI generation (images + speech). Nil or true → on;
	// false → no new generation happens (no provider/API calls) even with --generate-ai, while
	// any already-generated, committed assets are still served. The quick "turn it all off" button.
	Enabled *bool     `yaml:"enabled,omitempty"`
	Image   ImageGen  `yaml:"image,omitempty"`
	Speech  SpeechGen `yaml:"speech,omitempty"`
}

// Active reports whether AI generation is switched on (default true).
func (g Generation) Active() bool { return g.Enabled == nil || *g.Enabled }

// SpeechGen configures the text-to-speech generator that reads posts opting in with
// `audio: true`. Mirrors ImageGen: Provider selects a profile (driver/endpoint/default
// model/key-env); the rest overrides it. Voice is the default voice id; a post or its
// author/persona can override it.
type SpeechGen struct {
	Provider    string `yaml:"provider,omitempty"`    // minimax (more later)
	Model       string `yaml:"model,omitempty"`       // overrides the profile default
	Voice       string `yaml:"voice,omitempty"`       // default voice id
	Format      string `yaml:"format,omitempty"`      // audio format; default mp3 (podcast-portable)
	OutputDir   string `yaml:"output_dir,omitempty"`  // cache dir; default content/assets/generated
	BaseURL     string `yaml:"base_url,omitempty"`    // overrides the profile endpoint
	APIPath     string `yaml:"api_path,omitempty"`    // overrides the profile request path
	APIKey      string `yaml:"api_key,omitempty"`     // inline key, usually "{env:VAR}"; else profile env var
	Concurrency int    `yaml:"concurrency,omitempty"` // max clips generated in parallel; <=0 → default
	// Waveform precomputes an amplitude-peaks sidecar (decoding the audio) so the theme can draw
	// an accurate static waveform; nil/true → on. When off/undecodable, the player falls back to
	// a live Web Audio visualiser.
	Waveform *bool `yaml:"waveform,omitempty"`
	// Transcript controls how a post's content is turned into spoken text — what to do with
	// blocks that don't read well aloud (code, math, tables, diagrams).
	Transcript SpeechTranscript `yaml:"transcript,omitempty"`
}

// SpeechTranscript configures the prose extraction that feeds text-to-speech. Blocks maps a
// block type to a handling: "cue" (replace with a spoken cue), "drop" (remove silently),
// "keep" (read its text aloud), or — for inline_code only — "spell" (read symbols as words,
// so a path like /etc/foo is voiced "slash etc slash foo" instead of "divided by"). WrapUp
// appends a closing note when any block was cued. Recognised types: code, math_display,
// math_inline, table, diagram, inline_code. Callouts and blockquotes are always read.
type SpeechTranscript struct {
	Blocks map[string]string `yaml:"blocks,omitempty"`
	WrapUp *bool             `yaml:"wrap_up,omitempty"`
	// ExpandAcronyms reads acronym glossary terms as their expansion in speech (SSH → "Secure
	// Shell") so they're spoken as words, not letters. Nil/true → on. Only glossary entries that
	// look like acronym expansions are affected (an all-caps term with a short Title-Case
	// definition whose letters spell the acronym); ordinary terms like "Rust" are left alone.
	ExpandAcronyms *bool `yaml:"expand_acronyms,omitempty"`
}

// defaultSpeechBlocks: the visual blocks default to a spoken cue; inline math is dropped
// (it sits mid-sentence); inline code is spelled out (paths/flags read intelligibly).
// Anything unlisted is kept (read aloud).
var defaultSpeechBlocks = map[string]string{
	"code": "cue", "math_display": "cue", "math_inline": "drop", "table": "cue", "diagram": "cue",
	"inline_code": "spell",
}

// Block returns the handling for a block type, applying the defaults for omitted keys.
func (t SpeechTranscript) Block(typ string) string {
	if t.Blocks != nil {
		if b, ok := t.Blocks[typ]; ok {
			switch b = strings.ToLower(strings.TrimSpace(b)); b {
			case "cue", "drop", "keep", "spell":
				return b
			}
		}
	}
	if d, ok := defaultSpeechBlocks[typ]; ok {
		return d
	}
	return "keep"
}

// WrapsUp reports whether to append the closing "visit the post" note (default true).
func (t SpeechTranscript) WrapsUp() bool { return t.WrapUp == nil || *t.WrapUp }

// ExpandsAcronyms reports whether acronym glossary terms are read as their expansion (default true).
func (t SpeechTranscript) ExpandsAcronyms() bool { return t.ExpandAcronyms == nil || *t.ExpandAcronyms }

// Configured reports whether a speech provider has been selected.
func (g SpeechGen) Configured() bool { return strings.TrimSpace(g.Provider) != "" }

// Waveforms reports whether a waveform-peaks sidecar should be generated (default true).
func (g SpeechGen) Waveforms() bool { return g.Waveform == nil || *g.Waveform }

// ImageGen configures the image generator that satisfies `gen:` references in
// frontmatter (hero:/image:) and content (![alt](<gen:…>)). Provider selects a
// built-in profile that supplies the driver, endpoint and default model; the
// remaining fields override the profile. The API key is taken from APIKey when set
// (typically a {env:VAR} reference) else from the profile's default env var, so the
// secret stays in the environment and is never required in the config file.
type ImageGen struct {
	Provider     string            `yaml:"provider,omitempty"`      // google | minimax | openai | together | deepinfra | custom
	Model        string            `yaml:"model,omitempty"`         // overrides the profile's default model
	OutputDir    string            `yaml:"output_dir,omitempty"`    // cache dir for images + sidecars; default content/assets/generated
	BaseURL      string            `yaml:"base_url,omitempty"`      // overrides the profile endpoint (required for `custom`)
	APIPath      string            `yaml:"api_path,omitempty"`      // overrides the profile request path (OpenAI-compatible hosts)
	APIKey       string            `yaml:"api_key,omitempty"`       // inline key, usually "{env:VAR}"; empty → profile's default env var
	Defaults     map[string]string `yaml:"defaults,omitempty"`      // default tuning params (e.g. aspect: "16:9") applied to every request
	Concurrency  int               `yaml:"concurrency,omitempty"`   // max images generated in parallel; <=0 → default
	SystemPrompt string            `yaml:"system_prompt,omitempty"` // house style, overrides the theme's; per-ref ?systemprompt= overrides this
	Postprocess  ImagePostprocess  `yaml:"postprocess,omitempty"`   // deterministic fixes applied to generated bytes
}

// ImagePostprocess configures deterministic fixes applied to a freshly generated image
// before it is cached (a namespace so future steps can be added).
type ImagePostprocess struct {
	// TrimLetterbox removes solid black letterbox/pillarbox bars some providers bake in, then
	// re-frames to the requested aspect. Nil/true → on (a no-op when there are no bars).
	TrimLetterbox *bool `yaml:"trim_letterbox,omitempty"`
}

// TrimsLetterbox reports whether letterbox trimming is enabled (default true).
func (p ImagePostprocess) TrimsLetterbox() bool { return p.TrimLetterbox == nil || *p.TrimLetterbox }

// Configured reports whether an image generator provider has been selected.
func (g ImageGen) Configured() bool { return strings.TrimSpace(g.Provider) != "" }
