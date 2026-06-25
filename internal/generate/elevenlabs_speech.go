package generate

import (
	"context"
	"fmt"
	"net/url"
)

// elevenLabsTextLimit caps per-request characters; longer text is chunked and the raw PCM
// concatenated (PCM samples join cleanly). eleven_multilingual_v2 accepts more, but a
// conservative cap keeps requests responsive and within model limits.
const elevenLabsTextLimit = 4000

// elevenLabsPCMRate is the sample rate of the PCM we request. output_format=pcm_16000 returns
// 16 kHz mono 16-bit little-endian PCM, matching the WAV pipeline and the MiniMax driver.
const (
	elevenLabsPCMRate      = 16000
	elevenLabsOutputFormat = "pcm_16000"
)

// ElevenLabs speech profile defaults (used by speechProfiles in speech.go).
const (
	elevenLabsBaseURL      = "https://api.elevenlabs.io/v1"
	elevenLabsAPIPath      = "/text-to-speech"
	elevenLabsDefaultModel = "eleven_multilingual_v2"
	elevenLabsDefaultVoice = "JBFqnCBsd6RMkjVDRZzb" // "George", a British-English premade voice
)

// elevenLabsSpeech drives the ElevenLabs text-to-speech API. Unlike MiniMax it returns the
// audio as the raw response body (not JSON), and the voice id is part of the URL path. The
// MiniMax-style pronunciation dictionary does not apply here — ElevenLabs uses a separate
// mechanism (phoneme tags / uploaded pronunciation dictionaries) — so req.Pronunciation is
// ignored by this driver.
type elevenLabsSpeech struct {
	endpoint string // https://api.elevenlabs.io/v1/text-to-speech
	apiKey   string
	locator  *elevenLabsLocator // IPA pronunciation dictionary version, when synced
}

func (d *elevenLabsSpeech) Generate(ctx context.Context, req SpeechRequest) (SpeechResult, error) {
	var audio []byte
	for _, chunk := range chunkText(req.Text, elevenLabsTextLimit) {
		b, err := d.synth(ctx, req, chunk)
		if err != nil {
			return SpeechResult{}, err
		}
		audio = append(audio, b...)
	}
	if len(audio) == 0 {
		return SpeechResult{}, fmt.Errorf("no audio returned")
	}
	return SpeechResult{Bytes: audio, MIME: "audio/L16", SampleRate: elevenLabsPCMRate}, nil
}

func (d *elevenLabsSpeech) synth(ctx context.Context, req SpeechRequest, text string) ([]byte, error) {
	// `say` respellings apply universally via text substitution. IPA entries need an uploaded
	// pronunciation dictionary (a follow-up); they are ignored here.
	text = applySayAliases(text, req.Pronunciation)
	u := d.endpoint + "/" + url.PathEscape(req.Voice) + "?output_format=" + elevenLabsOutputFormat
	body := map[string]any{"text": text, "model_id": req.Model}
	if d.locator != nil {
		body["pronunciation_dictionary_locators"] = []elevenLabsLocator{*d.locator}
	}
	headers := map[string]string{"xi-api-key": d.apiKey, "Accept": "audio/*"}
	return postRaw(ctx, u, headers, body)
}
