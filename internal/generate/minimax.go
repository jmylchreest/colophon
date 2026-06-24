package generate

import (
	"context"
	"encoding/base64"
	"fmt"
)

// minimaxDriver calls MiniMax's bespoke image_generation endpoint (it is not
// OpenAI-compatible). It requests base64 output and checks base_resp.status_code,
// which MiniMax uses to report errors even on HTTP 200.
type minimaxDriver struct {
	endpoint string
	apiKey   string
}

// minimaxStatusError turns a non-zero base_resp.status_code into an error, tagging the rate-limit
// codes (1002 RPM/concurrency, 1039 TPM) as retryable so the shared backoff layer kicks in.
func minimaxStatusError(code int, msg string) error {
	err := fmt.Errorf("minimax error %d: %s", code, msg)
	if code == 1002 || code == 1039 {
		return rateLimited(0, err) // MiniMax gives no Retry-After; the policy's backoff applies
	}
	return err
}

type minimaxResponse struct {
	Data struct {
		ImageBase64 []string `json:"image_base64"`
		ImageURLs   []string `json:"image_urls"`
	} `json:"data"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

func (d *minimaxDriver) Generate(ctx context.Context, req ImageRequest) (ImageResult, error) {
	body := map[string]any{
		"model":           req.Model,
		"prompt":          withSystem(req.System, req.Prompt),
		"n":               1,
		"response_format": "base64",
	}
	if a := req.Params["aspect"]; a != "" {
		body["aspect_ratio"] = a
	}
	var out minimaxResponse
	headers := map[string]string{"Authorization": "Bearer " + d.apiKey}
	if err := postJSON(ctx, d.endpoint, headers, body, &out); err != nil {
		return ImageResult{}, err
	}
	if out.BaseResp.StatusCode != 0 {
		return ImageResult{}, minimaxStatusError(out.BaseResp.StatusCode, out.BaseResp.StatusMsg)
	}
	if len(out.Data.ImageBase64) > 0 {
		raw, err := base64.StdEncoding.DecodeString(out.Data.ImageBase64[0])
		if err != nil {
			return ImageResult{}, fmt.Errorf("decode image data: %w", err)
		}
		return ImageResult{Bytes: raw, MIME: "image/png"}, nil
	}
	if len(out.Data.ImageURLs) > 0 {
		raw, mime, err := fetchBytes(ctx, out.Data.ImageURLs[0])
		if err != nil {
			return ImageResult{}, err
		}
		return ImageResult{Bytes: raw, MIME: mime}, nil
	}
	return ImageResult{}, fmt.Errorf("no image returned")
}
