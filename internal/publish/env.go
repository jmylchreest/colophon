package publish

import (
	"os"
	"strings"
)

// FirstEnv returns the trimmed value of the first set, non-empty environment variable among
// keys, or "" if none is set. Drivers use it to accept a primary credential var with provider
// aliases (e.g. R2_ACCESS_KEY_ID falling back to AWS_ACCESS_KEY_ID).
func FirstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}
