package config

import "testing"

func TestEnvironmentGated(t *testing.T) {
	yes, no := true, false
	tests := []struct {
		name  string
		allow *bool
		want  bool
	}{
		{"default (nil) is ungated", nil, false},
		{"true is ungated", &yes, false},
		{"false is gated", &no, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (Environment{AllowPublish: tt.allow}).Gated(); got != tt.want {
				t.Errorf("Gated() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPublisherMerged(t *testing.T) {
	base := PublisherConfig{ID: "cf", Driver: "cloudflare-pages", Settings: map[string]any{"project": "blog", "branch": "main"}}
	got := base.Merged(map[string]any{"branch": "preview"})

	if got.Settings["branch"] != "preview" {
		t.Errorf("branch override = %v, want preview", got.Settings["branch"])
	}
	if got.Settings["project"] != "blog" {
		t.Errorf("project = %v, want blog (unchanged)", got.Settings["project"])
	}
	if base.Settings["branch"] != "main" {
		t.Error("Merged mutated the original Settings")
	}
}
