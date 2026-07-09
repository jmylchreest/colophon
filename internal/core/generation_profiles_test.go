package core

import "testing"

func TestSpeechResolveProfile(t *testing.T) {
	wrapUp := false
	base := SpeechGen{
		Provider:          "elevenlabs",
		Voice:             "default-voice",
		Model:             "eleven_v3",
		PronunciationDict: PronunciationDicts{"": "en_GB"},
		Reuse:             "exact",
		Transcript:        SpeechTranscript{Blocks: map[string]string{"code": "cue"}, WrapUp: &wrapUp},
		Profiles: map[string]SpeechGen{
			"minimax": {Provider: "minimax", Voice: "mm-voice"},
		},
	}

	// default → the base block, sans profiles.
	got, err := base.ResolveProfile("")
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "elevenlabs" || got.Voice != "default-voice" || got.Profiles != nil {
		t.Errorf("default profile = %+v", got)
	}

	// named profile overrides provider+voice, inherits everything else.
	mm, err := base.ResolveProfile("minimax")
	if err != nil {
		t.Fatal(err)
	}
	if mm.Provider != "minimax" || mm.Voice != "mm-voice" {
		t.Errorf("minimax overrides not applied: %+v", mm)
	}
	if mm.Model != "eleven_v3" || mm.PronunciationDict[""] != "en_GB" {
		t.Errorf("minimax should inherit model/dict, got %+v", mm)
	}
	if mm.Transcript.Blocks["code"] != "cue" || mm.Transcript.WrapUp == nil || *mm.Transcript.WrapUp {
		t.Errorf("minimax should inherit transcript, got %+v", mm.Transcript)
	}
	if mm.Profiles != nil {
		t.Error("a resolved profile must carry no sub-profiles")
	}

	// unknown name is an error.
	if _, err := base.ResolveProfile("nope"); err == nil {
		t.Error("unknown speech profile should error")
	}
}

func TestImageResolveProfile(t *testing.T) {
	base := ImageGen{
		Provider: "google",
		Model:    "imagen-4",
		Defaults: map[string]string{"aspect": "16:9", "quality": "low"},
		Profiles: map[string]ImageGen{
			"poster": {Provider: "openai", Model: "gpt-image-1", Defaults: map[string]string{"quality": "high"}},
		},
	}

	p, err := base.ResolveProfile("poster")
	if err != nil {
		t.Fatal(err)
	}
	if p.Provider != "openai" || p.Model != "gpt-image-1" {
		t.Errorf("poster overrides not applied: %+v", p)
	}
	// Maps deep-merge: aspect inherited, quality overridden.
	if p.Defaults["aspect"] != "16:9" || p.Defaults["quality"] != "high" {
		t.Errorf("defaults should deep-merge, got %+v", p.Defaults)
	}
	// The base block's defaults must be untouched by the merge.
	if base.Defaults["quality"] != "low" {
		t.Errorf("base defaults mutated: %+v", base.Defaults)
	}
	if _, err := base.ResolveProfile("ghost"); err == nil {
		t.Error("unknown image profile should error")
	}
}
