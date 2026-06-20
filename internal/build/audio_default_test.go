package build

import (
	"testing"

	"github.com/jmylchreest/colophon/internal/core"
)

func TestAudioWantsDefault(t *testing.T) {
	on := &audioResolver{defaultAudioOn: true}
	off := &audioResolver{defaultAudioOn: false}
	tr, fa := true, false

	if !on.wantsAudio(nil) {
		t.Error("unset + site default on → should want audio")
	}
	if off.wantsAudio(nil) {
		t.Error("unset + site default off → should not want audio")
	}
	if !off.wantsAudio(&tr) {
		t.Error("explicit audio:true must win over a default-off site")
	}
	if on.wantsAudio(&fa) {
		t.Error("explicit audio:false must win over a default-on site")
	}
	var nilAR *audioResolver
	if nilAR.wantsAudio(&tr) {
		t.Error("a nil resolver never wants audio")
	}
}

func TestModalityEnabledDefaults(t *testing.T) {
	if !(core.ImageGen{}).On() || !(core.SpeechGen{}).On() {
		t.Error("modalities should default to on")
	}
	f := false
	if (core.ImageGen{Enabled: &f}).On() || (core.SpeechGen{Enabled: &f}).On() {
		t.Error("enabled:false should turn a modality off")
	}
}
