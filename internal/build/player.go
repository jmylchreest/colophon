package build

import _ "embed"

// playerJS is the shared audio-player enhancement, emitted to the site root (like
// search-ui.js) so ANY theme can offer the player with just markup + CSS — no per-theme
// JS copy. It upgrades a [data-audioplayer] block's native <audio> into a themed
// play/pause + scrubbable waveform, using a precomputed peaks sidecar when present and a
// live Web Audio visualiser otherwise.
//
//go:embed assets/player.js
var playerJS []byte

// emitPlayerAsset writes player.js to the output root when any page carries audio.
func emitPlayerAsset(write func(string, []byte) error, pages []page) error {
	for _, p := range pages {
		if p.HasAudio {
			return write("player.js", playerJS)
		}
	}
	return nil
}
