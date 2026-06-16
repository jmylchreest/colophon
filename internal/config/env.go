package config

import (
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

// interpolateEnv replaces {env:VAR} / {env:VAR:-default} placeholders in raw config
// bytes with environment values, resolved before YAML parsing so any string value
// may reference the environment. An unset variable with no default becomes empty.
func interpolateEnv(raw []byte) []byte {
	return envPattern.ReplaceAllFunc(raw, func(match []byte) []byte {
		m := envPattern.FindSubmatch(match)
		name, def := string(m[1]), m[2]
		if v, ok := os.LookupEnv(name); ok {
			return []byte(v)
		}
		return def
	})
}
