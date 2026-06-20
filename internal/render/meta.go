package render

import yaml "go.yaml.in/yaml/v3"

// themeMetaFile is the optional per-theme manifest, read with the same override/base
// layering as any other theme file.
const themeMetaFile = "theme.yaml"

// ThemeMeta is a theme's optional metadata (theme.yaml). It is descriptive, not
// behavioural: today only image generation reads it (for a house style), so unknown
// keys are ignored and a missing file yields the zero value.
type ThemeMeta struct {
	Description string     `yaml:"description"`
	Image       ThemeImage `yaml:"image"`
}

// ThemeImage groups image-related theme settings. GenAI namespaces the
// generation-time hints (system prompt today; attention/negative prompt/etc. later).
type ThemeImage struct {
	GenAI ThemeGenAI `yaml:"genai"`
}

// ThemeGenAI holds defaults applied to AI image generation for posts using this theme.
type ThemeGenAI struct {
	// SystemPrompt is the house style folded into every generated image's prompt unless
	// a post overrides or suppresses it. Empty falls back to the theme Description.
	SystemPrompt string `yaml:"system_prompt"`
}

// readMeta parses the theme's theme.yaml (across override/base layers), or the zero
// value when absent. A malformed file is treated as absent so it never breaks a build.
func (t *themeSource) readMeta() ThemeMeta {
	b, err := t.read(themeMetaFile)
	if err != nil {
		return ThemeMeta{}
	}
	var m ThemeMeta
	if err := yaml.Unmarshal(b, &m); err != nil {
		return ThemeMeta{}
	}
	return m
}
