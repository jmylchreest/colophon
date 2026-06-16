package core

import (
	"testing"
	"time"
)

func TestPublicationVisibleAt(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	tests := []struct {
		name         string
		draft        bool
		publishAfter *time.Time
		want         bool
	}{
		{"published, no embargo", false, nil, true},
		{"draft", true, nil, false},
		{"embargo passed", false, &past, true},
		{"embargo pending", false, &future, false},
		{"draft overrides passed embargo", true, &past, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Publication{Draft: tt.draft, PublishAfter: tt.publishAfter}
			if got := p.VisibleAt(now); got != tt.want {
				t.Errorf("VisibleAt = %v, want %v", got, tt.want)
			}
		})
	}
}
