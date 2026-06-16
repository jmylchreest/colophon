package config

import (
	"os"
	"regexp"
)

// envPattern matches {env:VAR} and {env:VAR:-default} placeholders.
var envPattern = regexp.MustCompile(`\{env:([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

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
