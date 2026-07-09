package generate

import (
	"context"
	"encoding/base64"
	"fmt"
)

// openaiDriver calls an OpenAI-compatible images endpoint (OpenAI, xAI, Together,
// DeepInfra, or any `custom` host). The image is returned either inline as base64
// (data[].b64_json) or as a URL to fetch (data[].url) — both are handled.
type openaiDriver struct {
	endpoint  string
	apiKey    string
	aspectKey string // when set, the `aspect` param is sent under this field (xAI: aspect_ratio)
}

type openaiResponse struct {
	Data []struct {
		B64JSON       string `json:"b64_json"`
		URL           string `json:"url"`
		RevisedPrompt string `json:"revised_prompt"`
	} `json:"data"`
}

func (d *openaiDriver) Generate(ctx context.Context, req ImageRequest) (ImageResult, error) {
	body := map[string]any{
		"model":  req.Model,
		"prompt": withSystem(req.System, req.Prompt),
		"n":      1,
	}
	if size := req.Params["size"]; size != "" {
		body["size"] = size
	}
	if a := req.Params["aspect"]; a != "" && d.aspectKey != "" {
		body[d.aspectKey] = a
	}
	var out openaiResponse
	headers := map[string]string{"Authorization": "Bearer " + d.apiKey}
	if err := postJSON(ctx, d.endpoint, headers, body, &out); err != nil {
		return ImageResult{}, err
	}
	if len(out.Data) == 0 {
		return ImageResult{}, fmt.Errorf("no image returned")
	}
	item := out.Data[0]
	if item.B64JSON != "" {
		raw, err := base64.StdEncoding.DecodeString(item.B64JSON)
		if err != nil {
			return ImageResult{}, fmt.Errorf("decode image data: %w", err)
		}
		return ImageResult{Bytes: raw, MIME: "image/png", RevisedPrompt: item.RevisedPrompt}, nil
	}
	if item.URL != "" {
		raw, mime, err := fetchBytes(ctx, item.URL)
		if err != nil {
			return ImageResult{}, err
		}
		return ImageResult{Bytes: raw, MIME: mime, RevisedPrompt: item.RevisedPrompt}, nil
	}
	return ImageResult{}, fmt.Errorf("response carried neither b64_json nor url")
}
