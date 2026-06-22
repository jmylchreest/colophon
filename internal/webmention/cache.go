package webmention

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// SentCache records, per source URL, the set of targets last notified. It makes re-runs
// correct: only newly-added targets are sent, and targets that were dropped from a post are
// re-sent (so the receiver re-fetches, sees the link gone, and removes its mention).
type SentCache struct {
	Sent map[string][]string `json:"sent"`
}

// LoadSentCache reads the cache file; a missing file yields an empty cache (not an error).
func LoadSentCache(path string) (*SentCache, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &SentCache{Sent: map[string][]string{}}, nil
	}
	if err != nil {
		return nil, err
	}
	c := &SentCache{}
	if err := json.Unmarshal(b, c); err != nil {
		return nil, err
	}
	if c.Sent == nil {
		c.Sent = map[string][]string{}
	}
	return c, nil
}

// Save writes the cache, creating parent dirs and pruning empty entries first.
func (c *SentCache) Save(path string) error {
	for src, ts := range c.Sent {
		if len(ts) == 0 {
			delete(c.Sent, src)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// Diff returns the targets added (in current, not previously sent) and dropped (previously
// sent, no longer present). Both need a webmention: added announces the new link, dropped
// re-pings so the receiver re-checks the (now-removed) link.
func Diff(previous, current []string) (added, dropped []string) {
	prev := toSet(previous)
	cur := toSet(current)
	for t := range cur {
		if _, ok := prev[t]; !ok {
			added = append(added, t)
		}
	}
	for t := range prev {
		if _, ok := cur[t]; !ok {
			dropped = append(dropped, t)
		}
	}
	sort.Strings(added)
	sort.Strings(dropped)
	return added, dropped
}

func toSet(xs []string) map[string]struct{} {
	m := make(map[string]struct{}, len(xs))
	for _, x := range xs {
		m[x] = struct{}{}
	}
	return m
}
