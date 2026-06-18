package config

import (
	"bytes"
	"os"
	"regexp"
	"sort"
)

// envPattern matches {env:VAR} and {env:VAR:-default} placeholders.
var envPattern = regexp.MustCompile(`\{env:([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

// envRefs returns the unique {env:VAR} variable names referenced in raw config bytes,
// sorted. It reports the names regardless of whether the variables are set, so `colophon
// env` can list everything a project depends on.
func envRefs(raw []byte) []string {
	set := map[string]struct{}{}
	for _, m := range envPattern.FindAllSubmatch(raw, -1) {
		set[string(m[1])] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// interpolateEnv replaces {env:VAR} / {env:VAR:-default} placeholders in raw config bytes
// with environment values, resolved before YAML parsing so any string value may reference
// the environment. An unset variable with no default becomes empty.
//
// Because substitution happens on the raw text, an injected value must be escaped for the
// YAML scalar it lands in — otherwise a value like `\~/vault` inside a "double-quoted"
// scalar is read as an invalid escape and the parse fails. The quoting context is detected
// per placeholder (see yamlEscape).
func interpolateEnv(raw []byte) []byte {
	locs := envPattern.FindAllSubmatchIndex(raw, -1)
	if locs == nil {
		return raw
	}
	var out []byte
	prev := 0
	for _, loc := range locs {
		start, end := loc[0], loc[1]
		name := raw[loc[2]:loc[3]]
		var val []byte
		if v, ok := os.LookupEnv(string(name)); ok {
			val = []byte(v)
		} else if loc[4] >= 0 { // {env:VAR:-default} with the default group present
			val = raw[loc[4]:loc[5]]
		}
		out = append(out, raw[prev:start]...)
		out = append(out, yamlEscape(raw, start, val)...)
		prev = end
	}
	return append(out, raw[prev:]...)
}

// yamlEscape escapes val for the YAML scalar quoting active at byte offset pos. It scans the
// current line up to pos to decide whether the placeholder sits inside a double-quoted scalar
// (backslashes/quotes/control chars need escaping), a single-quoted scalar (a quote is escaped
// by doubling), or a plain scalar (returned unchanged — plain YAML treats backslash literally).
func yamlEscape(raw []byte, pos int, val []byte) []byte {
	prefix := raw[bytes.LastIndexByte(raw[:pos], '\n')+1 : pos]
	inDouble, inSingle := false, false
	for i := 0; i < len(prefix); i++ {
		switch c := prefix[i]; {
		case inDouble:
			if c == '\\' {
				i++ // skip the escaped char
			} else if c == '"' {
				inDouble = false
			}
		case inSingle:
			if c == '\'' {
				inSingle = false
			}
		case c == '"':
			inDouble = true
		case c == '\'':
			inSingle = true
		}
	}
	switch {
	case inDouble:
		return escapeDoubleQuoted(val)
	case inSingle:
		return bytes.ReplaceAll(val, []byte("'"), []byte("''"))
	default:
		return val
	}
}

// escapeDoubleQuoted escapes val for a YAML double-quoted scalar: backslash and quote are
// backslash-escaped, and the common control characters use their YAML escape sequences.
func escapeDoubleQuoted(val []byte) []byte {
	var b bytes.Buffer
	b.Grow(len(val))
	for i := 0; i < len(val); i++ {
		switch c := val[i]; c {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(c)
		}
	}
	return b.Bytes()
}
