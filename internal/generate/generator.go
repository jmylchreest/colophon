package generate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// New constructs the image generator for these settings, dispatching on the
// resolved driver. It fails when the API key is missing, so callers can warn and
// skip generation rather than discovering the gap mid-request.
func New(s Settings) (ImageGenerator, error) {
	if s.APIKey == "" {
		return nil, fmt.Errorf("provider %q: no API key (set api_key or the provider's env var)", s.Provider)
	}
	switch s.Driver {
	case driverGoogle:
		return &googleDriver{baseURL: s.BaseURL, apiKey: s.APIKey}, nil
	case driverOpenAI:
		if s.BaseURL == "" || s.APIPath == "" {
			return nil, fmt.Errorf("provider %q: base_url and api_path are required", s.Provider)
		}
		return &openaiDriver{endpoint: s.BaseURL + s.APIPath, apiKey: s.APIKey}, nil
	case driverMiniMax:
		return &minimaxDriver{endpoint: s.BaseURL + s.APIPath, apiKey: s.APIKey}, nil
	default:
		return nil, fmt.Errorf("unknown driver %q", s.Driver)
	}
}

// httpClient is shared by the HTTP drivers; image generation can be slow, so the
// timeout is generous.
var httpClient = &http.Client{Timeout: 180 * time.Second}

// postJSON sends body as JSON to url with the given headers and decodes the JSON
// response into out. A non-2xx status returns an error carrying the response body.
func postJSON(ctx context.Context, url string, headers map[string]string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", resp.Status, truncate(data, 400))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w (body: %s)", err, truncate(data, 200))
	}
	return nil
}

// fetchBytes GETs url and returns its body, for providers that return image URLs
// rather than inline base64.
func fetchBytes(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("fetch image: %s", resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	return b, resp.Header.Get("Content-Type"), err
}

// withSystem prepends a system/style prompt to the user prompt for providers that have
// no system role (MiniMax, OpenAI images, Imagen). Empty system returns the prompt
// unchanged. Gemini uses its native systemInstruction instead.
func withSystem(system, prompt string) string {
	if system == "" {
		return prompt
	}
	return prompt + "\n\n" + system
}

func truncate(b []byte, n int) string {
	if len(b) > n {
		return string(b[:n]) + "…"
	}
	return string(b)
}
