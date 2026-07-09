package generate

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIDriverAspectKey(t *testing.T) {
	png := base64.StdEncoding.EncodeToString([]byte("fake-png"))
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = map[string]any{}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"b64_json": png}}})
	}))
	defer srv.Close()

	req := ImageRequest{Prompt: "a lighthouse", Model: "grok-imagine-image-quality", Params: map[string]string{"aspect": "16:9"}}

	d := &openaiDriver{endpoint: srv.URL, apiKey: "k", aspectKey: "aspect_ratio"}
	if _, err := d.Generate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if got["aspect_ratio"] != "16:9" {
		t.Errorf("aspect_ratio = %v, want 16:9", got["aspect_ratio"])
	}

	d = &openaiDriver{endpoint: srv.URL, apiKey: "k"}
	if _, err := d.Generate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["aspect_ratio"]; ok {
		t.Error("aspect_ratio should not be sent when the profile has no aspectKey")
	}
}
