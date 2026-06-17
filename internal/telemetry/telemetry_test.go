package telemetry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/colophon/internal/core"
)

// TestDisabledClientIsNoop checks that an unconfigured or opted-out client never panics and
// emits nothing — call sites rely on being able to use it unconditionally.
func TestDisabledClientIsNoop(t *testing.T) {
	off := false
	tests := []struct {
		name      string
		telemetry core.Telemetry
		optOut    string
	}{
		{"no credentials", core.Telemetry{}, ""},
		{
			"master switch off",
			core.Telemetry{
				Enabled:      &off,
				Statsfactory: core.TelemetryStatsfactory{ServerURL: "https://s", AppKey: "sf_live_x"},
			},
			"",
		},
		{
			"opted out via env",
			core.Telemetry{Statsfactory: core.TelemetryStatsfactory{ServerURL: "https://s", AppKey: "sf_live_x"}},
			"off",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.optOut != "" {
				t.Setenv(envOptOut, tt.optOut)
			}
			c := New(tt.telemetry, "production", "test", t.TempDir())
			if c.enabled() {
				t.Fatalf("expected disabled client")
			}
			// None of these should panic or do anything observable.
			c.Build("default", 3)
			c.Source("md-dir", "content", 2)
			c.Publish("local", "local", "deployed", 5, 10)
			c.Flush()
		})
	}
}

// TestBakedDefaultEnablesClient checks that release-baked credentials enable the client even
// when the config leaves them empty (config values fall back to the baked defaults).
func TestBakedDefaultEnablesClient(t *testing.T) {
	DefaultServerURL = "https://baked.example.com"
	DefaultAppKey = "sf_live_baked"
	t.Cleanup(func() { DefaultServerURL, DefaultAppKey = "", "" })

	c := New(core.Telemetry{}, "production", "test", t.TempDir())
	if !c.enabled() {
		t.Fatal("expected an enabled client from baked defaults")
	}
	c.Flush()

	// The master switch still overrides baked defaults.
	off := false
	if New(core.Telemetry{Enabled: &off}, "production", "test", t.TempDir()).enabled() {
		t.Fatal("master switch off should disable even with baked defaults")
	}
}

func TestInstallIDStableAndHashed(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	id1 := installID(root)
	if len(id1) != 64 { // sha-256 hex
		t.Fatalf("install id = %q (len %d), want 64 hex chars", id1, len(id1))
	}
	if id2 := installID(root); id2 != id1 {
		t.Fatalf("install id not stable: %q != %q", id1, id2)
	}
	// The raw bytes are never persisted — only the hash lives in the id file.
	b, err := os.ReadFile(filepath.Join(root, ".colophon", "telemetry.id"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != id1 {
		t.Fatalf("stored id %q != returned %q", b, id1)
	}
}

func TestOptedOut(t *testing.T) {
	for _, v := range []string{"off", "false", "0", "no", "OFF"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv(envOptOut, v)
			if !optedOut() {
				t.Errorf("optedOut() = false for %q, want true", v)
			}
		})
	}
	t.Run("on", func(t *testing.T) {
		t.Setenv(envOptOut, "on")
		if optedOut() {
			t.Errorf("optedOut() = true for 'on', want false")
		}
	})
}
