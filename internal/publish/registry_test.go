package publish

import (
	"strings"
	"testing"
)

func TestDriverEnvVars(t *testing.T) {
	RegisterEnv("test-a", "FOO", "BAR")
	RegisterEnv("test-b", "BAR", "BAZ") // BAR overlaps and must dedupe

	got := strings.Join(DriverEnvVars([]string{"test-a", "test-b"}), ",")
	if got != "BAR,BAZ,FOO" { // sorted, unique
		t.Errorf("DriverEnvVars = %q, want BAR,BAZ,FOO", got)
	}
	if len(DriverEnvVars([]string{"unknown-driver"})) != 0 {
		t.Error("an unknown driver should contribute no env vars")
	}
	if len(DriverEnvVars(nil)) != 0 {
		t.Error("no drivers should yield no env vars")
	}
}
