package core

import (
	"fmt"
	"sort"
	"strings"
)

// defaultProfile is the implicit name of the unnamed (default) generation block, so
// `speech_profile: default` / `image_profile: default` explicitly select it.
const defaultProfile = "default"

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
	Enabled   *bool  `yaml:"enabled,omitempty"`    // per-modality guard + per-post audio default; nil/true → on
	Provider  string `yaml:"provider,omitempty"`   // minimax (more later)
	Model     string `yaml:"model,omitempty"`      // overrides the profile default
	Voice     string `yaml:"voice,omitempty"`      // default voice id
	OutputDir string `yaml:"output_dir,omitempty"` // cache dir; default content/assets/generated
	// PronunciationDict applies pronunciation dictionaries to generated readings, per language.
	// Each ref is either a built-in name shipped under contrib (e.g. "en_GB", "es_ES") or a path
	// (relative to site root) to a YAML dict ({pronunciations: [{word, ipa|say}]}); legacy JSON
	// tone/map files also load. A naked scalar (`pronunciation_dict: en_GB`) is sugar for the
	// site's default language; a map (`pronunciation_dict: {en: en_GB, es: es_ES}`) keys dicts by
	// BCP-47 tag, matched exact-then-base (es-MX → es). A post whose language has no entry gets
	// no dictionary — an English dict never rewrites a Spanish reading.
	PronunciationDict PronunciationDicts `yaml:"pronunciation_dict,omitempty"`
	// Reuse controls cache reuse when the renderer changes (provider/model/voice). "exact"
	// (default) re-renders on any change; "content" reuses an existing reading of the same text
	// (any prior voice) instead of re-rendering, so swapping providers doesn't regenerate.
	Reuse       string `yaml:"reuse,omitempty"`
	BaseURL     string `yaml:"base_url,omitempty"`    // overrides the profile endpoint
	APIPath     string `yaml:"api_path,omitempty"`    // overrides the profile request path
	APIKey      string `yaml:"api_key,omitempty"`     // inline key, usually "{env:VAR}"; else profile env var
	Concurrency int    `yaml:"concurrency,omitempty"` // max clips generated in parallel; <=0 → default
	// Transcript controls how a post's content is turned into spoken text — what to do with
	// blocks that don't read well aloud (code, math, tables, diagrams).
	Transcript SpeechTranscript `yaml:"transcript,omitempty"`
	// Profiles are named alternate speech configurations, selectable per environment or per
	// post via `speech_profile:`. Each inherits this (default) block and overrides only the
	// fields it sets; voice/model ids are provider-specific, so a profile that switches provider
	// should set its own. The unnamed block above is the implicit profile "default".
	Profiles map[string]SpeechGen `yaml:"profiles,omitempty"`
}

// ResolveProfile returns the speech config for a named profile: the default block with the
// named profile's overrides layered on (scalars replace, maps deep-merge). An empty name (or
// "default") returns the default block. An unknown name is an error listing what's configured.
func (g SpeechGen) ResolveProfile(name string) (SpeechGen, error) {
	if name = strings.TrimSpace(name); name == "" || name == defaultProfile {
		out := g
		out.Profiles = nil
		return out, nil
	}
	p, ok := g.Profiles[name]
	if !ok {
		return SpeechGen{}, fmt.Errorf("unknown speech profile %q (have: %s)", name, profileNames(g.Profiles))
	}
	return mergeSpeechGen(g, p), nil
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

// On reports whether speech generation is switched on (default true). It also serves as the
// per-post `audio:` default: with a provider configured and this on, posts read aloud by
// default unless they set `audio: false`.
func (g SpeechGen) On() bool { return g.Enabled == nil || *g.Enabled }

// ImageGen configures the image generator that satisfies `gen:` references in
// frontmatter (hero:/image:) and content (![alt](<gen:…>)). Provider selects a
// built-in profile that supplies the driver, endpoint and default model; the
// remaining fields override the profile. The API key is taken from APIKey when set
// (typically a {env:VAR} reference) else from the profile's default env var, so the
// secret stays in the environment and is never required in the config file.
type ImageGen struct {
	Enabled      *bool             `yaml:"enabled,omitempty"`       // per-modality guard; nil/true → on
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
	// Reuse controls cache reuse when the renderer changes (provider/model). "exact" (default)
	// re-generates when the provider/model differs from the cached image's sidecar; "content"
	// reuses any existing image for the same prompt/style/params instead of regenerating.
	Reuse string `yaml:"reuse,omitempty"`
	// Profiles are named alternate image configurations, selectable per environment or per post
	// via `image_profile:` (or per `gen:` ref via ?profile=). Each inherits this (default) block
	// and overrides only what it sets. The unnamed block above is the implicit profile "default".
	Profiles map[string]ImageGen `yaml:"profiles,omitempty"`
}

// ResolveProfile returns the image config for a named profile: the default block with the named
// profile's overrides layered on (scalars replace, maps deep-merge). An empty name (or "default")
// returns the default block. An unknown name is an error listing what's configured.
func (g ImageGen) ResolveProfile(name string) (ImageGen, error) {
	if name = strings.TrimSpace(name); name == "" || name == defaultProfile {
		out := g
		out.Profiles = nil
		return out, nil
	}
	p, ok := g.Profiles[name]
	if !ok {
		return ImageGen{}, fmt.Errorf("unknown image profile %q (have: %s)", name, profileNames(g.Profiles))
	}
	return mergeImageGen(g, p), nil
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

// On reports whether image generation is switched on (default true).
func (g ImageGen) On() bool { return g.Enabled == nil || *g.Enabled }

// pick returns a when it is non-blank, else b — so a profile's blank field inherits the default.
func pick(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// mergeStrMap layers ov over base (ov wins per key); nil only when both are empty.
func mergeStrMap(base, ov map[string]string) map[string]string {
	if base == nil && ov == nil {
		return nil
	}
	out := make(map[string]string, len(base)+len(ov))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range ov {
		out[k] = v
	}
	return out
}

// profileNames lists a profiles map's keys (sorted) for diagnostics.
func profileNames[T any](m map[string]T) string {
	if len(m) == 0 {
		return "(none)"
	}
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// mergeSpeechGen layers a profile (ov) over the default speech block (base): scalars replace when
// set, maps deep-merge, *bool/int override when present. The result carries no sub-profiles.
func mergeSpeechGen(base, ov SpeechGen) SpeechGen {
	out := base
	if ov.Enabled != nil {
		out.Enabled = ov.Enabled
	}
	out.Provider = pick(ov.Provider, base.Provider)
	out.Model = pick(ov.Model, base.Model)
	out.Voice = pick(ov.Voice, base.Voice)
	out.OutputDir = pick(ov.OutputDir, base.OutputDir)
	out.PronunciationDict = PronunciationDicts(mergeStrMap(base.PronunciationDict, ov.PronunciationDict))
	out.Reuse = pick(ov.Reuse, base.Reuse)
	out.BaseURL = pick(ov.BaseURL, base.BaseURL)
	out.APIPath = pick(ov.APIPath, base.APIPath)
	out.APIKey = pick(ov.APIKey, base.APIKey)
	if ov.Concurrency > 0 {
		out.Concurrency = ov.Concurrency
	}
	out.Transcript = mergeTranscript(base.Transcript, ov.Transcript)
	out.Profiles = nil
	return out
}

// mergeTranscript deep-merges block handlings and lets a profile flip the boolean toggles.
func mergeTranscript(base, ov SpeechTranscript) SpeechTranscript {
	out := base
	out.Blocks = mergeStrMap(base.Blocks, ov.Blocks)
	if ov.WrapUp != nil {
		out.WrapUp = ov.WrapUp
	}
	if ov.ExpandAcronyms != nil {
		out.ExpandAcronyms = ov.ExpandAcronyms
	}
	return out
}

// mergeImageGen layers a profile (ov) over the default image block (base), mirroring
// mergeSpeechGen. The result carries no sub-profiles.
func mergeImageGen(base, ov ImageGen) ImageGen {
	out := base
	if ov.Enabled != nil {
		out.Enabled = ov.Enabled
	}
	out.Provider = pick(ov.Provider, base.Provider)
	out.Model = pick(ov.Model, base.Model)
	out.OutputDir = pick(ov.OutputDir, base.OutputDir)
	out.BaseURL = pick(ov.BaseURL, base.BaseURL)
	out.APIPath = pick(ov.APIPath, base.APIPath)
	out.APIKey = pick(ov.APIKey, base.APIKey)
	out.SystemPrompt = pick(ov.SystemPrompt, base.SystemPrompt)
	out.Reuse = pick(ov.Reuse, base.Reuse)
	out.Defaults = mergeStrMap(base.Defaults, ov.Defaults)
	if ov.Concurrency > 0 {
		out.Concurrency = ov.Concurrency
	}
	if ov.Postprocess.TrimLetterbox != nil {
		out.Postprocess.TrimLetterbox = ov.Postprocess.TrimLetterbox
	}
	out.Profiles = nil
	return out
}
