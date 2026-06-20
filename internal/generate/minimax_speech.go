package generate

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
)

// minimaxTextLimit is a conservative per-request character cap; longer text is split into
// chunks whose audio is concatenated (MP3 frames concatenate cleanly for playback).
const minimaxTextLimit = 4000

type minimaxSpeech struct {
	endpoint string
	apiKey   string
}

type minimaxSpeechResponse struct {
	Data struct {
		Audio string `json:"audio"`
	} `json:"data"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

func (d *minimaxSpeech) Generate(ctx context.Context, req SpeechRequest) (SpeechResult, error) {
	var audio []byte
	for _, chunk := range chunkText(req.Text, minimaxTextLimit) {
		b, err := d.synth(ctx, req, chunk)
		if err != nil {
			return SpeechResult{}, err
		}
		audio = append(audio, b...)
	}
	if len(audio) == 0 {
		return SpeechResult{}, fmt.Errorf("no audio returned")
	}
	return SpeechResult{Bytes: audio, MIME: AudioMIME(req.Format)}, nil
}

func (d *minimaxSpeech) synth(ctx context.Context, req SpeechRequest, text string) ([]byte, error) {
	audioSetting := map[string]any{"format": req.Format, "sample_rate": 32000, "channel": 1}
	if req.Format == "mp3" {
		audioSetting["bitrate"] = 128000 // bitrate is mp3-only
	}
	body := map[string]any{
		"model":         req.Model,
		"text":          text,
		"voice_setting": map[string]any{"voice_id": req.Voice, "speed": 1.0, "vol": 1.0, "pitch": 0},
		"audio_setting": audioSetting,
	}
	var out minimaxSpeechResponse
	headers := map[string]string{"Authorization": "Bearer " + d.apiKey}
	if err := postJSON(ctx, d.endpoint, headers, body, &out); err != nil {
		return nil, err
	}
	if out.BaseResp.StatusCode != 0 {
		return nil, fmt.Errorf("minimax error %d: %s", out.BaseResp.StatusCode, out.BaseResp.StatusMsg)
	}
	if out.Data.Audio == "" {
		return nil, fmt.Errorf("no audio in response")
	}
	raw, err := hex.DecodeString(out.Data.Audio)
	if err != nil {
		return nil, fmt.Errorf("decode audio hex: %w", err)
	}
	return raw, nil
}

// chunkText splits text into pieces no longer than max characters, breaking on paragraph
// then sentence then word boundaries so speech doesn't cut mid-word.
func chunkText(text string, max int) []string {
	text = strings.TrimSpace(text)
	if len(text) <= max {
		return []string{text}
	}
	var chunks []string
	for len(text) > max {
		cut := lastBreak(text[:max])
		chunks = append(chunks, strings.TrimSpace(text[:cut]))
		text = strings.TrimSpace(text[cut:])
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}

// lastBreak finds a good split offset within s: the last paragraph break, else sentence
// end, else space, else the whole length.
func lastBreak(s string) int {
	for _, sep := range []string{"\n\n", ". ", "! ", "? ", "\n", " "} {
		if i := strings.LastIndex(s, sep); i > 0 {
			return i + len(sep)
		}
	}
	return len(s)
}
