// Package core holds colophon's domain model: the language-neutral types that the
// CLI, build pipeline, sources, and publishers all share. It deliberately has no
// dependencies on cobra, the filesystem layout, or any publisher driver so that
// it stays a clean library that other layers (and a future MCP wrapper) build on.
package core

// PersonaKind distinguishes a personal/individual identity from a shared brand.
//
// An individual persona maps to exactly one operator. A brand persona is the one
// sanctioned case where multiple operators may write as the same identity (e.g. a
// company PR team sharing one byline).
type PersonaKind string

const (
	PersonaIndividual PersonaKind = "individual"
	PersonaBrand      PersonaKind = "brand"
)

// HCard is the IndieWeb h-card identity for a persona. It drives bylines, author
// pages, feeds, and (later) microformats2 / fediverse federation.
type HCard struct {
	Name   string   `yaml:"name"`
	Bio    string   `yaml:"bio,omitempty"`
	Avatar string   `yaml:"avatar,omitempty"`
	Email  string   `yaml:"email,omitempty"`
	URLs   []string `yaml:"urls,omitempty"`
}

// Style is the optional voice/style profile for a persona. It is only consulted
// when content is generated with AI assistance; publishing content as-is needs
// none of it. The calling agent does the writing — colophon merely supplies this
// guide plus retrieved exemplars from the persona's own corpus.
type Style struct {
	// Guide is the freeform style/system prompt: tone, formatting rules, do/don'ts.
	Guide string `yaml:"guide,omitempty"`
	// References are links/glossaries/source docs the author may draw on.
	References []string `yaml:"references,omitempty"`
}

// Persona is a blog identity. Content is attributed to a persona, not to a human;
// the byline and the style corpus both attach here. A single human may own many
// personas (1:many); a persona normally maps to one human operator, except brand
// personas which may have several.
type Persona struct {
	ID          string      `yaml:"id"`
	DisplayName string      `yaml:"display_name"`
	Byline      string      `yaml:"byline,omitempty"`
	Kind        PersonaKind `yaml:"kind"`
	HCard       HCard       `yaml:"hcard"`
	Style       Style       `yaml:"style,omitempty"`

	// Sites lists the site IDs this persona is allowed to publish to. A site also
	// declares which personas it accepts; a publication must satisfy both.
	Sites []string `yaml:"sites"`
	// Operators are the human/agent identifiers permitted to write as this persona.
	// individual ⇒ exactly one; brand ⇒ one or more.
	Operators []string `yaml:"operators,omitempty"`
}

// Validate checks the persona's internal consistency.
func (p Persona) Validate() error {
	if p.ID == "" {
		return &ValidationError{Field: "id", Msg: "persona id is required"}
	}
	switch p.Kind {
	case PersonaIndividual:
		if len(p.Operators) > 1 {
			return &ValidationError{Field: "operators", Msg: "individual persona may have at most one operator (use kind: brand for shared)"}
		}
	case PersonaBrand:
		// any number of operators is fine
	case "":
		return &ValidationError{Field: "kind", Msg: "persona kind is required (individual|brand)"}
	default:
		return &ValidationError{Field: "kind", Msg: "unknown persona kind: " + string(p.Kind)}
	}
	return nil
}
