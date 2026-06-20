package generate

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
)

// googleDriver calls the Gemini Developer API (AI Studio) over plain HTTP. Gemini
// image models use the :generateContent endpoint (image returned inline as base64);
// Imagen models use :predict. The model name selects the path.
type googleDriver struct {
	baseURL string
	apiKey  string
}

func (d *googleDriver) Generate(ctx context.Context, req ImageRequest) (ImageResult, error) {
	if strings.HasPrefix(req.Model, "imagen-") {
		return d.generateImagen(ctx, req)
	}
	return d.generateGemini(ctx, req)
}

type geminiInlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text       string            `json:"text"`
				InlineData *geminiInlineData `json:"inlineData"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (d *googleDriver) generateGemini(ctx context.Context, req ImageRequest) (ImageResult, error) {
	body := map[string]any{
		"contents": []any{
			map[string]any{"parts": []any{map[string]any{"text": req.Prompt}}},
		},
		"generationConfig": geminiGenConfig(req.Params),
	}
	if req.System != "" {
		body["systemInstruction"] = map[string]any{"parts": []any{map[string]any{"text": req.System}}}
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent", strings.TrimRight(d.baseURL, "/"), req.Model)
	var out geminiResponse
	if err := postJSON(ctx, url, map[string]string{"x-goog-api-key": d.apiKey}, body, &out); err != nil {
		return ImageResult{}, err
	}
	for _, c := range out.Candidates {
		for _, p := range c.Content.Parts {
			if p.InlineData != nil && p.InlineData.Data != "" {
				raw, err := base64.StdEncoding.DecodeString(p.InlineData.Data)
				if err != nil {
					return ImageResult{}, fmt.Errorf("decode image data: %w", err)
				}
				return ImageResult{Bytes: raw, MIME: p.InlineData.MIMEType}, nil
			}
		}
	}
	return ImageResult{}, fmt.Errorf("no image returned (model %q may not support image output)", req.Model)
}

// geminiGenConfig builds generationConfig, always requesting image output and
// passing aspect ratio through imageConfig when supplied.
func geminiGenConfig(params map[string]string) map[string]any {
	cfg := map[string]any{"responseModalities": []string{"TEXT", "IMAGE"}}
	if a := params["aspect"]; a != "" {
		cfg["imageConfig"] = map[string]any{"aspectRatio": a}
	}
	return cfg
}

type imagenResponse struct {
	Predictions []struct {
		BytesBase64Encoded string `json:"bytesBase64Encoded"`
		MIMEType           string `json:"mimeType"`
	} `json:"predictions"`
}

func (d *googleDriver) generateImagen(ctx context.Context, req ImageRequest) (ImageResult, error) {
	params := map[string]any{"sampleCount": 1}
	if a := req.Params["aspect"]; a != "" {
		params["aspectRatio"] = a
	}
	body := map[string]any{
		"instances":  []any{map[string]any{"prompt": withSystem(req.System, req.Prompt)}},
		"parameters": params,
	}
	url := fmt.Sprintf("%s/v1beta/models/%s:predict", strings.TrimRight(d.baseURL, "/"), req.Model)
	var out imagenResponse
	if err := postJSON(ctx, url, map[string]string{"x-goog-api-key": d.apiKey}, body, &out); err != nil {
		return ImageResult{}, err
	}
	for _, p := range out.Predictions {
		if p.BytesBase64Encoded != "" {
			raw, err := base64.StdEncoding.DecodeString(p.BytesBase64Encoded)
			if err != nil {
				return ImageResult{}, fmt.Errorf("decode image data: %w", err)
			}
			return ImageResult{Bytes: raw, MIME: p.MIMEType}, nil
		}
	}
	return ImageResult{}, fmt.Errorf("no image returned")
}
