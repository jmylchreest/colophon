package cloudflare

import (
	"testing"
	"time"
)

func TestParsePrune(t *testing.T) {
	tests := []struct {
		in      any
		mode    pruneMode
		count   int
		age     time.Duration
		wantErr bool
	}{
		{"", pruneCount, 1, 0, false},
		{1, pruneCount, 1, 0, false},
		{5, pruneCount, 5, 0, false},
		{2.0, pruneCount, 2, 0, false},
		{"never", pruneNever, 0, 0, false},
		{"off", pruneNever, 0, 0, false},
		{false, pruneNever, 0, 0, false},
		{0, 0, 0, 0, true},
		{"0", 0, 0, 0, true},
		{"3w", pruneAge, 0, 3 * 7 * 24 * time.Hour, false},
		{"21d", pruneAge, 0, 21 * 24 * time.Hour, false},
		{"72h", pruneAge, 0, 72 * time.Hour, false},
		{"2 weeks", pruneAge, 0, 2 * 7 * 24 * time.Hour, false},
		{-1, 0, 0, 0, true},
		{"garbage", 0, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(pruneString(tt.in), func(t *testing.T) {
			got, err := parsePrune(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %v", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.mode != tt.mode || got.count != tt.count || got.age != tt.age {
				t.Errorf("parsePrune(%v) = %+v, want mode=%d count=%d age=%s", tt.in, got, tt.mode, tt.count, tt.age)
			}
		})
	}
}

func TestPruneToDelete(t *testing.T) {
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	mk := func(id string, ageDays int) deploymentInfo {
		return deploymentInfo{ID: id, CreatedOn: now.Add(-time.Duration(ageDays) * 24 * time.Hour)}
	}
	// newest-first
	deps := []deploymentInfo{mk("d0", 0), mk("d1", 5), mk("d2", 20), mk("d3", 40)}

	t.Run("count keeps newest N", func(t *testing.T) {
		del := pruneSpec{mode: pruneCount, count: 1}.toDelete(deps, now)
		if len(del) != 3 || del[0].ID != "d1" {
			t.Errorf("got %v", ids(del))
		}
	})

	t.Run("age keeps within window plus newest", func(t *testing.T) {
		// keep < 2 weeks: d0 (newest, always) and d1 (5d); delete d2 (20d), d3 (40d)
		del := pruneSpec{mode: pruneAge, age: 14 * 24 * time.Hour}.toDelete(deps, now)
		if got := ids(del); len(got) != 2 || got[0] != "d2" || got[1] != "d3" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("never deletes nothing", func(t *testing.T) {
		if del := (pruneSpec{mode: pruneNever}).toDelete(deps, now); len(del) != 0 {
			t.Errorf("got %v", ids(del))
		}
	})
}

func ids(ds []deploymentInfo) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.ID
	}
	return out
}
