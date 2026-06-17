package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// dotEnvFiles are loaded (in order) before {env:VAR} interpolation. A variable already set
// in the real environment is never overridden, and an earlier file wins over a later one, so
// the precedence is: real environment (e.g. CI secrets) > .env (local, gitignored) >
// .env.defaults (committed, shared non-secret defaults like a public analytics endpoint).
var dotEnvFiles = []string{".env", ".env.defaults"}

// loadDotEnv reads KEY=VALUE lines from each of root's dot-env files into the process
// environment, filling only variables that are not already set. It is best-effort: a missing
// file is skipped and a malformed line is ignored, so a project without any .env just uses
// the real environment and the config's {env:VAR:-default} fallbacks.
func loadDotEnv(root string) {
	for _, name := range dotEnvFiles {
		applyDotEnv(filepath.Join(root, name))
	}
}

func applyDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, val, ok := parseDotEnvLine(sc.Text())
		if !ok {
			continue
		}
		if _, set := os.LookupEnv(key); !set {
			_ = os.Setenv(key, val)
		}
	}
}

// parseDotEnvLine parses one `KEY=VALUE` line (a leading `export` is allowed, surrounding
// quotes on the value are stripped). It returns ok=false for blank lines, comments, and
// anything without a key before the `=`.
func parseDotEnvLine(line string) (key, val string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")
	eq := strings.IndexByte(line, '=')
	if eq <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:eq])
	val = strings.TrimSpace(line[eq+1:])
	if len(val) >= 2 {
		if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
			val = val[1 : len(val)-1]
		}
	}
	if key == "" {
		return "", "", false
	}
	return key, val, true
}
