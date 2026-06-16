package cloudflare

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// pruneSpec says how many old deployments to keep after a publish. It is one of:
//   - count: keep the newest N (N >= 1)
//   - age:   keep every deployment newer than a window (plus always the newest)
//   - never: keep everything
type pruneSpec struct {
	mode  pruneMode
	count int
	age   time.Duration
}

type pruneMode int

const (
	pruneNever pruneMode = iota
	pruneCount
	pruneAge
)

var durationRE = regexp.MustCompile(`^(\d+)\s*([a-z]+)$`)

// parsePrune reads a prune setting (a number, a duration, or "never"/"0"). An absent
// value is the caller's concern; an empty or "true" value means the default (keep 1).
func parsePrune(v any) (pruneSpec, error) {
	s := strings.TrimSpace(strings.ToLower(pruneString(v)))
	switch s {
	case "", "true":
		return pruneSpec{mode: pruneCount, count: 1}, nil
	case "never", "off", "none", "all", "false":
		return pruneSpec{mode: pruneNever}, nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		// Cloudflare never deletes the live deployment for a branch, so the floor is 1
		// ("keep only the newest"); 0 is rejected as ambiguous rather than overloaded.
		if n < 1 {
			return pruneSpec{}, fmt.Errorf("count must be >= 1 (use 1 to keep only the newest, or \"never\" to keep all)")
		}
		return pruneSpec{mode: pruneCount, count: n}, nil
	}
	if d, err := parsePruneDuration(s); err == nil {
		return pruneSpec{mode: pruneAge, age: d}, nil
	}
	return pruneSpec{}, fmt.Errorf("%q is not a count, a duration (e.g. 3w, 21d, 72h), or \"never\"", s)
}

// toDelete picks which deployments to remove, given this branch's deployments sorted
// newest-first. The newest is always kept (it holds the live branch alias).
func (p pruneSpec) toDelete(newestFirst []deploymentInfo, now time.Time) []deploymentInfo {
	switch p.mode {
	case pruneCount:
		if len(newestFirst) <= p.count {
			return nil
		}
		return newestFirst[p.count:]
	case pruneAge:
		cutoff := now.Add(-p.age)
		var del []deploymentInfo
		for i, d := range newestFirst {
			if i == 0 {
				continue // always keep the newest
			}
			if d.CreatedOn.Before(cutoff) {
				del = append(del, d)
			}
		}
		return del
	default: // pruneNever
		return nil
	}
}

func pruneString(v any) string {
	switch n := v.(type) {
	case string:
		return n
	case bool:
		if n {
			return "true"
		}
		return "never"
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case float64:
		return strconv.FormatInt(int64(n), 10)
	default:
		return fmt.Sprint(v)
	}
}

// parsePruneDuration accepts Go durations (72h) plus day/week units, and a leading
// number with a spelled-out unit ("3 weeks").
func parsePruneDuration(s string) (time.Duration, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	m := durationRE.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	n, _ := strconv.Atoi(m[1])
	switch m[2] {
	case "h", "hour", "hours":
		return time.Duration(n) * time.Hour, nil
	case "d", "day", "days":
		return time.Duration(n) * 24 * time.Hour, nil
	case "w", "week", "weeks":
		return time.Duration(n) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown duration unit %q", m[2])
	}
}
