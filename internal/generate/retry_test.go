package generate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// testPolicy is a retry policy that never actually sleeps, for fast deterministic tests.
func testPolicy(maxAttempts int) RetryPolicy {
	return RetryPolicy{
		MaxAttempts: maxAttempts, Base: time.Second, Max: time.Second,
		sleep: func(context.Context, time.Duration) error { return nil },
	}
}

func TestRetrySucceedsAfterRateLimits(t *testing.T) {
	calls := 0
	var waits int
	p := testPolicy(5)
	p.OnRetry = func(int, time.Duration, error) { waits++ }
	err := retry(context.Background(), p, func() error {
		calls++
		if calls < 3 {
			return rateLimited(0, errors.New("429"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("should eventually succeed, got %v", err)
	}
	if calls != 3 {
		t.Errorf("want 3 calls, got %d", calls)
	}
	if waits != 2 {
		t.Errorf("want 2 backoffs logged, got %d", waits)
	}
}

func TestRetryGivesUpAndAnnotates(t *testing.T) {
	calls := 0
	err := retry(context.Background(), testPolicy(4), func() error {
		calls++
		return rateLimited(0, errors.New("minimax error 1039: rate limit exceeded(TPM)"))
	})
	if calls != 4 {
		t.Fatalf("want 4 attempts, got %d", calls)
	}
	if err == nil || !strings.Contains(err.Error(), "gave up after 4 attempts") {
		t.Errorf("give-up error should note the attempt count, got %v", err)
	}
	if !strings.Contains(err.Error(), "1039") {
		t.Errorf("give-up error should preserve the underlying message, got %v", err)
	}
}

func TestRetryDoesNotRetryNonRateLimit(t *testing.T) {
	calls := 0
	sentinel := errors.New("decode failed")
	err := retry(context.Background(), testPolicy(5), func() error {
		calls++
		return sentinel
	})
	if calls != 1 {
		t.Errorf("non-rate-limit errors must fail fast; got %d calls", calls)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("want the original error, got %v", err)
	}
}

func TestRetryDisabledPolicyOneShot(t *testing.T) {
	for _, n := range []int{0, 1} {
		calls := 0
		p := RetryPolicy{MaxAttempts: n}
		if p.enabled() {
			t.Errorf("MaxAttempts=%d should be disabled", n)
		}
		err := retry(context.Background(), p, func() error {
			calls++
			return rateLimited(0, errors.New("429"))
		})
		if calls != 1 {
			t.Errorf("MaxAttempts=%d should make exactly one call, got %d", n, calls)
		}
		// A disabled policy returns the raw rate-limit error, not the "gave up" annotation.
		if err == nil || strings.Contains(err.Error(), "gave up") {
			t.Errorf("disabled policy should return the raw error, got %v", err)
		}
	}
}

func TestRetryHonoursContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := RetryPolicy{
		MaxAttempts: 5, Base: time.Second, Max: time.Second,
		sleep: func(context.Context, time.Duration) error { cancel(); return context.Canceled },
	}
	calls := 0
	err := retry(ctx, p, func() error {
		calls++
		return rateLimited(0, errors.New("429"))
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("want context.Canceled, got %v", err)
	}
	if calls != 1 {
		t.Errorf("should stop after the cancelled wait; got %d calls", calls)
	}
}

func TestMinimaxStatusErrorClassifies(t *testing.T) {
	for _, code := range []int{1002, 1039} {
		if _, ok := asRateLimit(minimaxStatusError(code, "x")); !ok {
			t.Errorf("code %d should be a rate limit", code)
		}
	}
	for _, code := range []int{1004, 2013, 1000} {
		if _, ok := asRateLimit(minimaxStatusError(code, "x")); ok {
			t.Errorf("code %d should NOT be a rate limit", code)
		}
	}
}

func TestRetryAfterHintWins(t *testing.T) {
	// A provider Retry-After longer than the computed backoff is honoured.
	p := RetryPolicy{MaxAttempts: 3, Base: time.Second, Max: time.Second}
	got := p.backoff(1, 30*time.Second)
	if got != 30*time.Second {
		t.Errorf("retry-after should win when larger; got %s", got)
	}
}
