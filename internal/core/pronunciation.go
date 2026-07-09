package core

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// PronunciationDicts maps a language tag to a pronunciation dictionary ref. The "" key holds the
// naked-scalar form's ref and means "the site's default language" — resolved against it at build
// time, when the default is known. See SpeechGen.PronunciationDict for the YAML forms.
type PronunciationDicts map[string]string

// UnmarshalYAML accepts the two config forms: a naked scalar (stored under the "" key) or a
// map of language tag → ref (keys lower-cased, blank refs dropped).
func (p *PronunciationDicts) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		var s string
		if err := n.Decode(&s); err != nil {
			return err
		}
		if s = strings.TrimSpace(s); s != "" {
			*p = PronunciationDicts{"": s}
		}
		return nil
	case yaml.MappingNode:
		m := map[string]string{}
		if err := n.Decode(&m); err != nil {
			return err
		}
		out := PronunciationDicts{}
		for k, v := range m {
			if v = strings.TrimSpace(v); v != "" {
				out[strings.ToLower(strings.TrimSpace(k))] = v
			}
		}
		if len(out) > 0 {
			*p = out
		}
		return nil
	}
	return fmt.Errorf("pronunciation_dict: expected a dictionary ref or a {lang: ref} map")
}

// UnmarshalText accepts the naked-scalar form on the config-loader path (koanf/mapstructure
// decodes scalars through encoding.TextUnmarshaler; UnmarshalYAML above covers direct YAML).
func (p *PronunciationDicts) UnmarshalText(b []byte) error {
	if s := strings.TrimSpace(string(b)); s != "" {
		*p = PronunciationDicts{"": s}
	}
	return nil
}

// For returns the dictionary ref for a post language: an exact tag match wins, then the base
// language (es-MX → es), then — only when the post is in the site's default language — the
// naked-scalar entry. Languages with no entry get "" (no dictionary).
func (p PronunciationDicts) For(lang, defLang string) string {
	if len(p) == 0 {
		return ""
	}
	l := strings.ToLower(strings.TrimSpace(lang))
	d := strings.ToLower(strings.TrimSpace(defLang))
	if l == "" {
		l = d
	}
	if v, ok := p.get(l); ok {
		return v
	}
	if b := baseTag(l); b != l {
		if v, ok := p.get(b); ok {
			return v
		}
	}
	if baseTag(l) == baseTag(d) {
		v, _ := p.get("")
		return v
	}
	return ""
}

// get looks a key up case-insensitively: the config-loader path (mapstructure) delivers map
// keys as authored, so `ES: es_ES` must still match a post in "es".
func (p PronunciationDicts) get(key string) (string, bool) {
	if v, ok := p[key]; ok {
		return v, true
	}
	for k, v := range p {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return "", false
}

// baseTag returns a BCP-47 tag's primary subtag (es-mx → es).
func baseTag(tag string) string {
	if i := strings.IndexByte(tag, '-'); i > 0 {
		return tag[:i]
	}
	return tag
}
