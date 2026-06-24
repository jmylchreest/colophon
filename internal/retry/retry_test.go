package retry

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// testPolicy never actually sleeps, for fast deterministic tests.
func testPolicy(maxAttempts int) Policy {
	return Policy{
		MaxAttempts: maxAttempts, Base: time.Second, Max: time.Second,
		sleep: func(context.Context, time.Duration) error { return nil },
	}
}

func TestDoSucceedsAfterRetryables(t *testing.T) {
	calls, waits := 0, 0
	p := testPolicy(5)
	p.OnRetry = func(int, time.Duration, error) { waits++ }
	err := Do(context.Background(), p, func() error {
		calls++
		if calls < 3 {
			return RateLimited(0, errors.New("429"))
		}
		return nil
	})
	if err != nil || calls != 3 || waits != 2 {
		t.Fatalf("err=%v calls=%d waits=%d", err, calls, waits)
	}
}

func TestDoGivesUpAndAnnotates(t *testing.T) {
	calls := 0
	err := Do(context.Background(), testPolicy(4), func() error {
		calls++
		return Transient(errors.New("503 unavailable"))
	})
	if calls != 4 {
		t.Fatalf("want 4 attempts, got %d", calls)
	}
	if err == nil || !strings.Contains(err.Error(), "gave up after 4 attempts") || !strings.Contains(err.Error(), "503") {
		t.Errorf("give-up error = %v", err)
	}
}

func TestDoFailsFastOnNonRetryable(t *testing.T) {
	calls := 0
	sentinel := errors.New("decode failed")
	err := Do(context.Background(), testPolicy(5), func() error { calls++; return sentinel })
	if calls != 1 || !errors.Is(err, sentinel) {
		t.Errorf("calls=%d err=%v", calls, err)
	}
}

func TestDoDisabledPolicyOneShot(t *testing.T) {
	for _, n := range []int{0, 1} {
		calls := 0
		err := Do(context.Background(), Policy{MaxAttempts: n}, func() error {
			calls++
			return RateLimited(0, errors.New("429"))
		})
		if calls != 1 || err == nil || strings.Contains(err.Error(), "gave up") {
			t.Errorf("n=%d calls=%d err=%v", n, calls, err)
		}
	}
}

func TestDoHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := Policy{MaxAttempts: 5, Base: time.Second, Max: time.Second,
		sleep: func(context.Context, time.Duration) error { cancel(); return context.Canceled }}
	calls := 0
	err := Do(ctx, p, func() error { calls++; return Transient(errors.New("x")) })
	if !errors.Is(err, context.Canceled) || calls != 1 {
		t.Errorf("calls=%d err=%v", calls, err)
	}
}

// SafeRetryable (non-idempotent ops) retries rate-limit + unsent, but NOT ambiguous transients.
func TestSafeRetryableExcludesTransient(t *testing.T) {
	if _, ok := SafeRetryable(Transient(errors.New("x"))); ok {
		t.Error("transient must not be safe-retryable for non-idempotent ops")
	}
	for _, e := range []error{RateLimited(0, errors.New("x")), Unsent(errors.New("x"))} {
		if _, ok := SafeRetryable(e); !ok {
			t.Errorf("%v should be safe-retryable", e)
		}
	}
	// A SafeRetryable policy stops at the first transient (no retries), but rides out rate limits.
	calls := 0
	p := testPolicy(4)
	p.Retryable = SafeRetryable
	_ = Do(context.Background(), p, func() error { calls++; return Transient(errors.New("503")) })
	if calls != 1 {
		t.Errorf("safe policy should not retry transient; got %d calls", calls)
	}
}

func TestFromStatus(t *testing.T) {
	if _, ok := AsRetryable(FromStatus(429, time.Second, errors.New("x"))); !ok {
		t.Error("429 should be retryable")
	}
	if _, ok := AsRetryable(FromStatus(503, 0, errors.New("x"))); !ok {
		t.Error("503 should be retryable")
	}
	if _, ok := SafeRetryable(FromStatus(429, 0, errors.New("x"))); !ok {
		t.Error("429 should be safe-retryable")
	}
	if _, ok := SafeRetryable(FromStatus(503, 0, errors.New("x"))); ok {
		t.Error("503 should NOT be safe-retryable")
	}
	if _, ok := AsRetryable(FromStatus(404, 0, errors.New("x"))); ok {
		t.Error("404 should not be retryable")
	}
}

func TestFromDoErrClassifiesDialAsUnsent(t *testing.T) {
	dial := &net.OpError{Op: "dial", Err: errors.New("connection refused")}
	if _, ok := SafeRetryable(FromDoErr(dial)); !ok {
		t.Error("a dial failure should be Unsent (safe-retryable)")
	}
	dns := &net.DNSError{Err: "no such host"}
	if _, ok := SafeRetryable(FromDoErr(dns)); !ok {
		t.Error("a DNS failure should be Unsent (safe-retryable)")
	}
	// A read-phase failure is transient (retryable for idempotent, not safe for non-idempotent).
	readErr := &net.OpError{Op: "read", Err: errors.New("connection reset")}
	if _, ok := AsRetryable(FromDoErr(readErr)); !ok {
		t.Error("a read failure should be retryable")
	}
	if _, ok := SafeRetryable(FromDoErr(readErr)); ok {
		t.Error("a read failure should NOT be safe-retryable")
	}
}

func TestRetryAfterHeader(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "12")
	if got := RetryAfter(h); got != 12*time.Second {
		t.Errorf("RetryAfter = %s", got)
	}
	if got := RetryAfter(http.Header{}); got != 0 {
		t.Errorf("absent Retry-After = %s", got)
	}
}

func TestBackoffRespectsRetryAfter(t *testing.T) {
	p := Policy{MaxAttempts: 3, Base: time.Second, Max: time.Second}
	if got := p.backoff(1, 30*time.Second); got != 30*time.Second {
		t.Errorf("retry-after should win when larger; got %s", got)
	}
}
