package generate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	dicts "github.com/jmylchreest/colophon/contrib/pronunciation"
	yaml "go.yaml.in/yaml/v3"
)

// ResolvePronunciationDict loads the dictionary referenced by ref. A bare token — one that does
// not look like a path (no separator and no extension), e.g. "en_GB" — names a built-in shipped
// under contrib/pronunciation. Anything else is treated as a file path, resolved relative to
// root when not absolute, so a site can ship its own dictionary.
func ResolvePronunciationDict(ref, root string) ([]Pronunciation, error) {
	if !strings.ContainsAny(ref, `/\.`) {
		entries, ok, err := BuiltinPronunciationDict(ref)
		if !ok {
			return nil, fmt.Errorf("unknown built-in pronunciation dict %q (have: %s)", ref, strings.Join(BuiltinPronunciationDicts(), ", "))
		}
		return entries, err
	}
	p := ref
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, filepath.FromSlash(p))
	}
	return LoadPronunciationDict(p)
}

// BuiltinPronunciationDict loads a dictionary shipped under contrib/pronunciation by name
// (e.g. "en_GB"). ok is false when no built-in by that name exists, so the caller can fall
// back to treating the reference as a file path.
func BuiltinPronunciationDict(name string) (entries []Pronunciation, ok bool, err error) {
	b, readErr := dicts.FS.ReadFile(name + ".yaml")
	if readErr != nil {
		return nil, false, nil
	}
	entries, err = parsePronunciationYAML(b)
	return entries, true, err
}

// BuiltinPronunciationDicts lists the names of the dictionaries shipped under contrib.
func BuiltinPronunciationDicts() []string {
	ents, _ := dicts.FS.ReadDir(".")
	out := make([]string, 0, len(ents))
	for _, e := range ents {
		if name := strings.TrimSuffix(e.Name(), ".yaml"); name != e.Name() {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Pronunciation is one provider-agnostic override: speak Word using either IPA phonemes or a
// plain-text respelling (Say). Exactly one of IPA/Say is set. Each speech driver renders these
// into its own mechanism (MiniMax pronunciation_dict tone, ElevenLabs dictionary / text alias).
type Pronunciation struct {
	Word string
	IPA  string
	Say  string
}

// LoadPronunciationDict reads a pronunciation dictionary. YAML (.yaml/.yml) is the canonical,
// provider-agnostic format:
//
//	pronunciations:
//	  - word: schedule
//	    ipa: ʃˈɛdjuːl
//	  - word: nginx
//	    say: engine x
//
// Legacy JSON is still accepted: {"tone":["word/(ipa)","word/text"]} or a {word: replacement}
// object (a "(…)" replacement is read as IPA, plain text as a respelling).
func LoadPronunciationDict(path string) ([]Pronunciation, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return parsePronunciationYAML(b)
	default:
		return parsePronunciationJSON(b, path)
	}
}

func parsePronunciationYAML(b []byte) ([]Pronunciation, error) {
	var doc struct {
		Pronunciations []struct {
			Word string `yaml:"word"`
			IPA  string `yaml:"ipa"`
			Say  string `yaml:"say"`
		} `yaml:"pronunciations"`
	}
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("pronunciation dict: %w", err)
	}
	out := make([]Pronunciation, 0, len(doc.Pronunciations))
	for _, e := range doc.Pronunciations {
		if w := strings.TrimSpace(e.Word); w != "" {
			out = append(out, Pronunciation{Word: w, IPA: strings.TrimSpace(e.IPA), Say: strings.TrimSpace(e.Say)})
		}
	}
	return out, nil
}

func parsePronunciationJSON(b []byte, path string) ([]Pronunciation, error) {
	var wrapped struct {
		Tone []string `json:"tone"`
	}
	if err := json.Unmarshal(b, &wrapped); err == nil && len(wrapped.Tone) > 0 {
		out := make([]Pronunciation, 0, len(wrapped.Tone))
		for _, t := range wrapped.Tone {
			out = append(out, toneToPronunciation(t))
		}
		return out, nil
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err == nil && len(m) > 0 {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]Pronunciation, 0, len(m))
		for _, k := range keys {
			out = append(out, toneToPronunciation(k+"/"+m[k]))
		}
		return out, nil
	}
	return nil, fmt.Errorf("pronunciation dict %s: expected YAML, {\"tone\":[...]}, or a {word: replacement} object", path)
}

// toneToPronunciation parses a MiniMax tone entry "word/replacement" into a neutral entry: a
// "(…)" replacement is IPA, anything else is a respelling.
func toneToPronunciation(tone string) Pronunciation {
	word, repl := tone, ""
	if i := strings.IndexByte(tone, '/'); i >= 0 {
		word, repl = tone[:i], tone[i+1:]
	}
	word = strings.TrimSpace(word)
	repl = strings.TrimSpace(repl)
	if strings.HasPrefix(repl, "(") && strings.HasSuffix(repl, ")") {
		return Pronunciation{Word: word, IPA: strings.TrimSpace(repl[1 : len(repl)-1])}
	}
	return Pronunciation{Word: word, Say: repl}
}

// FilterPronunciation returns only the entries whose Word occurs in text as a whole word,
// case-insensitively (so "route" is not applied inside "router"). This keeps each request's
// payload small and the cache key tight.
func FilterPronunciation(dict []Pronunciation, text string) []Pronunciation {
	if len(dict) == 0 {
		return nil
	}
	lower := strings.ToLower(text)
	var out []Pronunciation
	for _, p := range dict {
		if w := strings.ToLower(strings.TrimSpace(p.Word)); w != "" && containsWord(lower, w) {
			out = append(out, p)
		}
	}
	return out
}

// minimaxTone renders entries to MiniMax pronunciation_dict "tone" strings.
func minimaxTone(ps []Pronunciation) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		switch {
		case p.IPA != "":
			out = append(out, p.Word+"/("+p.IPA+")")
		case p.Say != "":
			out = append(out, p.Word+"/"+p.Say)
		}
	}
	return out
}

// applySayAliases substitutes Say respellings into text as whole words, for providers without
// a pronunciation-dictionary mechanism. IPA-only entries are left for the provider to handle.
func applySayAliases(text string, ps []Pronunciation) string {
	for _, p := range ps {
		if p.Say != "" {
			text = replaceWholeWord(text, p.Word, p.Say)
		}
	}
	return text
}

// pronunciationKey is a stable digest of the applied entries for the cache key.
func pronunciationKey(ps []Pronunciation) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = p.Word + "|" + p.IPA + "|" + p.Say
	}
	return strings.Join(parts, "\n")
}

// containsWord reports whether word occurs in text bounded by non-word characters. Both must be
// lower-cased. Bytes ≥ 0x80 (lead/continuation bytes of accented letters) count as word chars.
func containsWord(text, word string) bool {
	for from := 0; ; {
		i := strings.Index(text[from:], word)
		if i < 0 {
			return false
		}
		i += from
		if !wordByte(text, i-1) && !wordByte(text, i+len(word)) {
			return true
		}
		from = i + 1
	}
}

// replaceWholeWord replaces whole-word, case-insensitive occurrences of word in text with repl.
func replaceWholeWord(text, word, repl string) string {
	if word == "" {
		return text
	}
	lower := strings.ToLower(text)
	lword := strings.ToLower(word)
	var b strings.Builder
	for from := 0; ; {
		i := strings.Index(lower[from:], lword)
		if i < 0 {
			b.WriteString(text[from:])
			break
		}
		i += from
		if !wordByte(lower, i-1) && !wordByte(lower, i+len(lword)) {
			b.WriteString(text[from:i])
			b.WriteString(repl)
			from = i + len(lword)
		} else {
			b.WriteString(text[from : i+1])
			from = i + 1
		}
	}
	return b.String()
}

func wordByte(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	b := s[i]
	return b == '_' || b >= 0x80 || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}
