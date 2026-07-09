package generate

import (
	"testing"

	"github.com/jmylchreest/colophon/internal/core"
)

func TestResolveProfileDefaults(t *testing.T) {
	t.Setenv("MINIMAX_API_KEY", "secret-key")
	s, err := Resolve(core.ImageGen{Provider: "minimax"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Driver != driverMiniMax {
		t.Errorf("driver = %q, want %q", s.Driver, driverMiniMax)
	}
	if s.Model != "image-01" {
		t.Errorf("model = %q, want image-01", s.Model)
	}
	if s.BaseURL != "https://api.minimax.io/v1" || s.APIPath != "/image_generation" {
		t.Errorf("endpoint = %q%q", s.BaseURL, s.APIPath)
	}
	if s.APIKey != "secret-key" {
		t.Errorf("APIKey should come from MINIMAX_API_KEY, got %q", s.APIKey)
	}
	if s.OutputDir != DefaultOutputDir {
		t.Errorf("OutputDir = %q, want default", s.OutputDir)
	}
}

func TestResolveExplicitOverrides(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "env-key")
	s, err := Resolve(core.ImageGen{
		Provider: "google",
		Model:    "imagen-4.0-fast-generate-001",
		APIKey:   "inline-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Model != "imagen-4.0-fast-generate-001" {
		t.Errorf("explicit model not honoured: %q", s.Model)
	}
	if s.APIKey != "inline-key" {
		t.Errorf("inline api_key should win over env, got %q", s.APIKey)
	}
}

func TestResolveXAIProfile(t *testing.T) {
	t.Setenv("XAI_API_KEY", "xai-key")
	s, err := Resolve(core.ImageGen{Provider: "xai"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Driver != driverOpenAI {
		t.Errorf("xai should use the OpenAI-compatible driver, got %q", s.Driver)
	}
	if s.Model != "grok-imagine-image-quality" {
		t.Errorf("model = %q, want grok-imagine-image-quality", s.Model)
	}
	if s.BaseURL != "https://api.x.ai/v1" || s.APIPath != "/images/generations" {
		t.Errorf("endpoint = %q%q", s.BaseURL, s.APIPath)
	}
	if s.APIKey != "xai-key" {
		t.Errorf("APIKey should come from XAI_API_KEY, got %q", s.APIKey)
	}
	if s.AspectKey != "aspect_ratio" {
		t.Errorf("AspectKey = %q, want aspect_ratio", s.AspectKey)
	}
}

func TestResolveCustomProvider(t *testing.T) {
	s, err := Resolve(core.ImageGen{
		Provider: "custom",
		Model:    "some-model",
		BaseURL:  "https://example.test/v1",
		APIPath:  "/images/generations",
		APIKey:   "k",
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.Driver != driverOpenAI {
		t.Errorf("custom should use the OpenAI-compatible driver, got %q", s.Driver)
	}
}

func TestResolveErrors(t *testing.T) {
	if _, err := Resolve(core.ImageGen{}); err == nil {
		t.Error("empty provider should error")
	}
	if _, err := Resolve(core.ImageGen{Provider: "nope"}); err == nil {
		t.Error("unknown provider should error")
	}
}

func TestNewRequiresKey(t *testing.T) {
	if _, err := New(Settings{Provider: "minimax", Driver: driverMiniMax, Model: "image-01"}); err == nil {
		t.Error("New without an API key should error")
	}
}
