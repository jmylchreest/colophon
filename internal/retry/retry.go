// Package retry is the shared rate-limit/transient backoff used by every outbound HTTP
// subsystem (AI generation, webmention, websub, syndication). Failures are classified into
// three kinds so callers can choose how much to retry:
//
//   - RateLimited — the request was refused before doing anything (HTTP 429, or a provider
//     body code like MiniMax 1039). Always safe to retry.
//   - Unsent — the request provably never reached the server (DNS failure, dial refused).
//     Always safe to retry, even for non-idempotent operations.
//   - Transient — an ambiguous mid-flight failure (5xx, a timeout or reset after the request
//     was sent). Safe to retry only for idempotent operations, since the server may have
//     already acted (e.g. a syndication POST that actually created the post).
//
// Do retries any kind; a Policy may set Retryable: SafeRetryable to retry only the first two,
// which is how non-idempotent calls (create-a-post) avoid duplicates.
package retry

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type kind int

const (
	kindRateLimit kind = iota
	kindTransient
	kindUnsent
)

// retryableError tags an error as retryable and carries an optional provider Retry-After.
type retryableError struct {
	kind  kind
	after time.Duration
	err   error
}

func (e *retryableError) Error() string { return e.err.Error() }
func (e *retryableError) Unwrap() error { return e.err }

// RateLimited / Transient / Unsent wrap err as the corresponding retryable kind.
func RateLimited(after time.Duration, err error) error {
	return &retryableError{kind: kindRateLimit, after: after, err: err}
}
func Transient(err error) error { return &retryableError{kind: kindTransient, err: err} }
func Unsent(err error) error    { return &retryableError{kind: kindUnsent, err: err} }

func asKind(err error) (*retryableError, bool) {
	var r *retryableError
	if errors.As(err, &r) {
		return r, true
	}
	return nil, false
}

// AsRetryable reports whether err is any retryable kind (the default predicate for idempotent ops).
func AsRetryable(err error) (time.Duration, bool) {
	if r, ok := asKind(err); ok {
		return r.after, true
	}
	return 0, false
}

// SafeRetryable reports whether err is safe to retry for a NON-idempotent op: a rate limit or a
// provably-unsent request, but not an ambiguous transient that may have already been processed.
func SafeRetryable(err error) (time.Duration, bool) {
	if r, ok := asKind(err); ok && (r.kind == kindRateLimit || r.kind == kindUnsent) {
		return r.after, true
	}
	return 0, false
}

// Policy controls backoff. The zero value does not retry. Retryable selects which errors qualify
// (defaults to AsRetryable); OnRetry, when set, is called before each wait so callers can log it.
type Policy struct {
	MaxAttempts int
	Base        time.Duration
	Max         time.Duration
	OnRetry     func(attempt int, wait time.Duration, err error)
	Retryable   func(error) (time.Duration, bool)
	sleep       func(context.Context, time.Duration) error // nil → sleepCtx; overridable in tests
}

// Default rides out a provider's rate-limit window without hanging a build: ~6 tries with
// exponential backoff (2s → 45s cap), giving up after roughly two minutes.
func Default() Policy {
	return Policy{MaxAttempts: 6, Base: 2 * time.Second, Max: 45 * time.Second}
}

// Enabled reports whether the policy actually retries.
func (p Policy) Enabled() bool { return p.MaxAttempts > 1 }

// backoff is the delay before the next try after attempt n: exponential from Base, capped at Max,
// with ±20% jitter so concurrent jobs don't resynchronise. A longer provider Retry-After wins.
func (p Policy) backoff(n int, retryAfter time.Duration) time.Duration {
	d := p.Base << (n - 1)
	if d <= 0 || d > p.Max {
		d = p.Max
	}
	jitter := time.Duration(rand.Int63n(int64(d)/5*2+1)) - d/5 // ±20%
	d += jitter
	if retryAfter > d {
		d = retryAfter
	}
	if d < 0 {
		d = 0
	}
	return d
}

// Do runs fn, retrying qualifying failures with backoff until the policy is exhausted or the
// context is cancelled. The final error is annotated with the attempt count when it gave up;
// non-qualifying errors return immediately.
func Do(ctx context.Context, p Policy, fn func() error) error {
	pred := p.Retryable
	if pred == nil {
		pred = AsRetryable
	}
	attempts := p.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}
	sleep := p.sleep
	if sleep == nil {
		sleep = sleepCtx
	}
	start := nowFunc()
	var err error
	for n := 1; n <= attempts; n++ {
		if err = fn(); err == nil {
			return nil
		}
		after, ok := pred(err)
		if !ok || n == attempts {
			break
		}
		wait := p.backoff(n, after)
		if p.OnRetry != nil {
			p.OnRetry(n, wait, err)
		}
		if cerr := sleep(ctx, wait); cerr != nil {
			return cerr
		}
	}
	if _, ok := pred(err); ok && attempts > 1 {
		return fmt.Errorf("%w (gave up after %d attempts over %s)", err, attempts, nowFunc().Sub(start).Round(time.Second))
	}
	return err
}

// sleepCtx waits d, returning early if the context is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// nowFunc is time.Now, overridable in tests.
var nowFunc = time.Now

// ---- HTTP classification helpers ----------------------------------------------------------

// RetryAfter reads a Retry-After header (delta-seconds form), 0 when absent/unparsable.
func RetryAfter(h http.Header) time.Duration {
	if v := strings.TrimSpace(h.Get("Retry-After")); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return 0
}

// FromStatus classifies a non-2xx HTTP status: 429 → rate-limited, 5xx → transient, else plain.
func FromStatus(code int, after time.Duration, err error) error {
	switch {
	case code == http.StatusTooManyRequests:
		return RateLimited(after, err)
	case code >= 500:
		return Transient(err)
	default:
		return err
	}
}

// FromDoErr classifies an http.Client.Do error: a DNS or dial failure provably never sent the
// request (Unsent), anything else (timeout/reset, possibly after sending) is Transient.
func FromDoErr(err error) error {
	if err == nil {
		return nil
	}
	var dnsErr *net.DNSError
	var opErr *net.OpError
	if errors.As(err, &dnsErr) || (errors.As(err, &opErr) && opErr.Op == "dial") {
		return Unsent(err)
	}
	return Transient(err)
}
