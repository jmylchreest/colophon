// Package core holds colophon's domain model: the language-neutral types that the
// CLI, build pipeline, sources, and publishers all share. It deliberately has no
// dependencies on cobra, the filesystem layout, or any publisher driver so that
// it stays a clean library that other layers (and a future MCP wrapper) build on.
package core

// Style is a persona's writing voice. It is only consulted when content is generated with
// AI assistance; publishing as-is needs none of it. The calling agent does the writing —
// colophon merely supplies this guide plus retrieved exemplars from the persona's corpus.
type Style struct {
	// Guide is the freeform style/character prompt: who the voice is and how it writes
	// (tone, formatting rules, do/don'ts) — e.g. "Senior engineer, lots of experience".
	Guide string `yaml:"guide,omitempty" json:"guide,omitempty"`
	// References are links/glossaries/source docs the voice may draw on.
	References []string `yaml:"references,omitempty" json:"references,omitempty"`
}

// Persona is a hidden writing VOICE — a character/style the agent writes in. It is never
// shown to readers (the byline is the Author); it exists to keep a consistent voice and to
// retrieve in-voice exemplars. Personas live in personas/*.yaml and can be shared across
// authors: a persona's corpus is every post written in it, regardless of who authored it.
type Persona struct {
	ID string `yaml:"id" json:"id"`
	// Name is a human label for the voice (not shown), e.g. "Senior engineer".
	Name  string `yaml:"name,omitempty" json:"name,omitempty"`
	Style Style  `yaml:"style,omitempty" json:"style,omitempty"`
	// Voice is the text-to-speech voice id for audio of posts written as this persona, used
	// when the author has none. A post's audio_voice or its author's voice takes precedence.
	Voice string `yaml:"voice,omitempty" json:"voice,omitempty"`
}

// Validate checks the persona's internal consistency.
func (p Persona) Validate() error {
	if p.ID == "" {
		return &ValidationError{Field: "id", Msg: "persona id is required"}
	}
	return nil
}
