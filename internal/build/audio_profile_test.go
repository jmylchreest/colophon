package build

import (
	"testing"

	"github.com/jmylchreest/colophon/internal/clog"
	"github.com/jmylchreest/colophon/internal/core"
)

// A post's speech_profile overrides the environment's, which overrides the default block.
func TestAudioResolvedProfileSelection(t *testing.T) {
	speech := core.SpeechGen{
		Provider: "elevenlabs",
		Voice:    "ev",
		Profiles: map[string]core.SpeechGen{
			"mm": {Provider: "minimax", Voice: "mmv"},
		},
	}
	noVoice := func(string, string, string) string { return "" }
	// The environment selects "mm" by default.
	ar := newAudioResolver(speech, "mm", t.TempDir(), "/", "http://example.test", nil, false, false, noVoice, "en", nil, true, clog.Discard())

	// A post that names nothing inherits the environment's profile (minimax).
	if rs := ar.resolved(""); rs.resolveErr != nil || rs.provider != "minimax" {
		t.Errorf("env profile should select minimax, got provider=%q err=%v", rs.provider, rs.resolveErr)
	}
	// A post can override back to the default block explicitly.
	if rs := ar.resolved("default"); rs.resolveErr != nil || rs.provider != "elevenlabs" {
		t.Errorf("post 'default' should select elevenlabs, got provider=%q err=%v", rs.provider, rs.resolveErr)
	}
	// An unknown profile resolves to an error (TTS skipped for that post), not a panic.
	if rs := ar.resolved("ghost"); rs.resolveErr == nil {
		t.Error("unknown profile should resolve to an error")
	}
}
