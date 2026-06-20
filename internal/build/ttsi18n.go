package build

import (
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// ttsI18nJSON is the built-in translation table for the text colophon injects into spoken
// audio (block cues, the "visit the post" hint, the wrap-up, and inline-code symbol names).
// A project can override or extend it with i18n/tts/replacements.json.
//
//go:embed assets/tts-i18n.json
var ttsI18nJSON []byte

// ttsStrings is the injected-speech vocabulary for one language. Templated entries carry a
// single %s for the language/diagram-kind descriptor.
type ttsStrings struct {
	Code        string            `json:"code"`
	CodeLang    string            `json:"code_lang"`
	Diagram     string            `json:"diagram"`
	DiagramKind string            `json:"diagram_kind"`
	Table       string            `json:"table"`
	Formula     string            `json:"formula"`
	Hint        string            `json:"hint"`
	WrapUp      string            `json:"wrap_up"`
	Listen      string            `json:"listen"` // player UI: figcaption
	Play        string            `json:"play"`   // player UI: play button aria-label
	Pause       string            `json:"pause"`  // player UI: pause button aria-label
	Symbols     map[string]string `json:"symbols"`
}

type ttsTable map[string]ttsStrings

var embeddedTTS = parseTTS(ttsI18nJSON)

func parseTTS(b []byte) ttsTable {
	t := ttsTable{}
	_ = json.Unmarshal(b, &t)
	return t
}

// loadTTSTable returns the embedded translations with a project i18n/tts/replacements.json
// merged over them (per-language, per-field) when present.
func loadTTSTable(root string) ttsTable {
	out := ttsTable{}
	for k, v := range embeddedTTS {
		out[k] = v
	}
	if root == "" {
		return out
	}
	b, err := os.ReadFile(filepath.Join(root, "i18n", "tts", "replacements.json"))
	if err != nil {
		return out
	}
	var over ttsTable
	if json.Unmarshal(b, &over) != nil {
		return out
	}
	for lang, o := range over {
		out[lang] = mergeTTS(out[lang], o)
	}
	return out
}

func mergeTTS(base, o ttsStrings) ttsStrings {
	base.Code = pick(o.Code, base.Code)
	base.CodeLang = pick(o.CodeLang, base.CodeLang)
	base.Diagram = pick(o.Diagram, base.Diagram)
	base.DiagramKind = pick(o.DiagramKind, base.DiagramKind)
	base.Table = pick(o.Table, base.Table)
	base.Formula = pick(o.Formula, base.Formula)
	base.Hint = pick(o.Hint, base.Hint)
	base.WrapUp = pick(o.WrapUp, base.WrapUp)
	base.Listen = pick(o.Listen, base.Listen)
	base.Play = pick(o.Play, base.Play)
	base.Pause = pick(o.Pause, base.Pause)
	if len(o.Symbols) > 0 {
		m := map[string]string{}
		for k, v := range base.Symbols {
			m[k] = v
		}
		for k, v := range o.Symbols {
			m[k] = v
		}
		base.Symbols = m
	}
	return base
}

// For resolves the strings for a language (base-language match, e.g. "es-MX" → "es"), filling
// any missing language or empty field from English.
func (t ttsTable) For(lang string) ttsStrings {
	en := t["en"]
	s := en
	if base := baseLang(lang); base != "" {
		if v, ok := t[base]; ok {
			s = v
		}
	}
	s.Code = pick(s.Code, en.Code)
	s.CodeLang = pick(s.CodeLang, en.CodeLang)
	s.Diagram = pick(s.Diagram, en.Diagram)
	s.DiagramKind = pick(s.DiagramKind, en.DiagramKind)
	s.Table = pick(s.Table, en.Table)
	s.Formula = pick(s.Formula, en.Formula)
	s.Hint = pick(s.Hint, en.Hint)
	s.WrapUp = pick(s.WrapUp, en.WrapUp)
	s.Listen = pick(s.Listen, en.Listen)
	s.Play = pick(s.Play, en.Play)
	s.Pause = pick(s.Pause, en.Pause)
	if len(s.Symbols) == 0 {
		s.Symbols = en.Symbols
	}
	return s
}

func baseLang(l string) string {
	l = strings.ToLower(strings.TrimSpace(l))
	if i := strings.IndexAny(l, "-_"); i > 0 {
		l = l[:i]
	}
	return l
}

func pick(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
