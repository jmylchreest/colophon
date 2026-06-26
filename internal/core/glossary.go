package core

import "go.yaml.in/yaml/v3"

// GlossaryLink is a reference link shown beside a decorated glossary term. The decorator renders
// these as citation-style superscripts after the term's first occurrence (a single link as a
// glyph, several as numbers); each is a real anchor, so they work without an interactive popover.
type GlossaryLink struct {
	Label string `yaml:"label" json:"label"`
	URL   string `yaml:"url" json:"url"`
}

// GlossaryEntry is a term's definition plus optional reference links. In glossary.yaml an entry
// is written either as a bare string (just the definition) or as a mapping with `definition` and
// `links:` — both decode through UnmarshalYAML, so the simple flat form keeps working unchanged.
type GlossaryEntry struct {
	Definition string         `yaml:"definition" json:"def"`
	Links      []GlossaryLink `yaml:"links,omitempty" json:"links,omitempty"`
}

// UnmarshalYAML accepts either a scalar (the definition alone) or a mapping {definition, links}.
func (e *GlossaryEntry) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&e.Definition)
	}
	type raw GlossaryEntry // shed the custom unmarshaler to decode the mapping form
	return node.Decode((*raw)(e))
}
