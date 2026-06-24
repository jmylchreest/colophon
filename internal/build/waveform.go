package build

import (
	"encoding/binary"
	"math"
)

// waveformBuckets is how many amplitude bars a precomputed waveform carries — enough detail
// for a header-width visual without bloating the sidecar. The peaks are spaced uniformly
// across the whole clip, so bar i maps linearly to time (i/N)·duration for accurate scrubbing.
const waveformBuckets = 120

// peaksFromWAV reads 16-bit PCM from a RIFF/WAVE file (for recorded .wav attachments),
// averaging channels to mono. Other recorded formats return ok=false → live visualiser.
func peaksFromWAV(b []byte) ([]float64, bool) {
	s, ok := decodeWAV(b)
	if !ok {
		return nil, false
	}
	return peaksFromSamples(s)
}

func peaksFromSamples(samples []int16) ([]float64, bool) {
	if len(samples) < waveformBuckets {
		return nil, false
	}
	peaks := make([]float64, waveformBuckets)
	per := float64(len(samples)) / float64(waveformBuckets)
	var maxPeak float64
	for i := range peaks {
		lo, hi := int(float64(i)*per), int(float64(i+1)*per)
		var m int
		for _, s := range samples[lo:hi] {
			v := int(s)
			if v < 0 {
				v = -v
			}
			if v > m {
				m = v
			}
		}
		peaks[i] = float64(m)
		if peaks[i] > maxPeak {
			maxPeak = peaks[i]
		}
	}
	if maxPeak == 0 {
		return nil, false
	}
	for i := range peaks {
		peaks[i] = math.Round(peaks[i]/maxPeak*1000) / 1000 // normalise + trim precision for a small sidecar
	}
	return peaks, true
}

// decodeWAV returns mono 16-bit samples from a RIFF/WAVE file's data chunk, or ok=false if
// it isn't 16-bit PCM.
func decodeWAV(b []byte) ([]int16, bool) {
	if len(b) < 44 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, false
	}
	var channels uint16 = 1
	var bits uint16 = 16
	p := 12
	for p+8 <= len(b) {
		id := string(b[p : p+4])
		size := int(binary.LittleEndian.Uint32(b[p+4 : p+8]))
		body := p + 8
		if body+size > len(b) {
			size = len(b) - body
		}
		switch id {
		case "fmt ":
			if size >= 16 {
				channels = binary.LittleEndian.Uint16(b[body+2 : body+4])
				bits = binary.LittleEndian.Uint16(b[body+14 : body+16])
			}
		case "data":
			if bits != 16 || channels == 0 {
				return nil, false
			}
			return wavSamples(b[body:body+size], int(channels)), true
		}
		p = body + size
		if size%2 == 1 {
			p++ // chunks are word-aligned
		}
	}
	return nil, false
}

func wavSamples(data []byte, channels int) []int16 {
	frame := 2 * channels
	out := make([]int16, 0, len(data)/frame)
	for i := 0; i+frame <= len(data); i += frame {
		sum := 0
		for c := 0; c < channels; c++ {
			sum += int(int16(binary.LittleEndian.Uint16(data[i+c*2:])))
		}
		out = append(out, int16(sum/channels))
	}
	return out
}
