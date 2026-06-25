package build

import "encoding/binary"

// ttsOutputExt and ttsOutputMIME are the delivery format for generated speech: WAV. It is
// lossless, universally browser-playable, and needs no encoder dependency — the provider
// returns raw PCM and we wrap it here. The player derives the waveform from this audio in
// the browser (see assets/player.js), so no peaks are precomputed server-side.
const (
	ttsOutputExt  = ".wav"
	ttsOutputMIME = "audio/wav"
)

// pcmToSamples interprets b as a stream of mono 16-bit little-endian signed PCM samples.
func pcmToSamples(b []byte) []int16 {
	s := make([]int16, len(b)/2)
	for i := range s {
		s[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
	}
	return s
}

// encodeWAV wraps mono 16-bit PCM samples in a minimal RIFF/WAV container.
func encodeWAV(samples []int16, sampleRate int) []byte {
	dataBytes := len(samples) * 2
	buf := make([]byte, 44+dataBytes)
	put32 := func(off int, v uint32) { binary.LittleEndian.PutUint32(buf[off:], v) }
	put16 := func(off int, v uint16) { binary.LittleEndian.PutUint16(buf[off:], v) }
	copy(buf[0:], "RIFF")
	put32(4, uint32(36+dataBytes))
	copy(buf[8:], "WAVEfmt ")
	put32(16, 16) // fmt chunk size
	put16(20, 1)  // PCM
	put16(22, 1)  // mono
	put32(24, uint32(sampleRate))
	put32(28, uint32(sampleRate*2)) // byte rate = sampleRate × channels × bytesPerSample
	put16(32, 2)                    // block align = channels × bytesPerSample
	put16(34, 16)                   // bits per sample
	copy(buf[36:], "data")
	put32(40, uint32(dataBytes))
	for i, s := range samples {
		put16(44+i*2, uint16(s))
	}
	return buf
}
