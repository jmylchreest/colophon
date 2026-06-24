package generate

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"
)

// rateLimitError marks a provider response as a retryable rate limit (HTTP 429, or a body-level
// code like MiniMax 1039/1002). retryAfter is the provider's hint when given (else 0). It wraps
// the underlying error so non-retry callers still see a useful message.
type rateLimitError struct {
	retryAfter time.Duration
	err        error
}

func (e *rateLimitError) Error() string { return e.err.Error() }
func (e *rateLimitError) Unwrap() error { return e.err }

// rateLimited wraps err as a retryable rate-limit error (retryAfter optional, 0 = none).
func rateLimited(retryAfter time.Duration, err error) error {
	return &rateLimitError{retryAfter: retryAfter, err: err}
}

// asRateLimit reports whether err (or anything it wraps) is a rate limit, with its retry-after.
func asRateLimit(err error) (time.Duration, bool) {
	var rl *rateLimitError
	if errors.As(err, &rl) {
		return rl.retryAfter, true
	}
	return 0, false
}

// RetryPolicy controls rate-limit backoff for provider calls. The zero value means no retries
// (fail fast). OnRetry, when set, is called before each wait so the caller can log the backoff.
type RetryPolicy struct {
	MaxAttempts int           // total tries including the first; <=1 disables retrying
	Base        time.Duration // first backoff delay
	Max         time.Duration // per-delay cap
	OnRetry     func(attempt int, wait time.Duration, err error)
	sleep       func(context.Context, time.Duration) error // nil → sleepCtx; overridable in tests
}

// DefaultRetryPolicy rides out a provider's rate-limit window without hanging a build: ~6 tries
// with exponential backoff (2s → 45s cap), giving up after roughly two minutes per call.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 6, Base: 2 * time.Second, Max: 45 * time.Second}
}

// enabled reports whether the policy actually retries.
func (p RetryPolicy) enabled() bool { return p.MaxAttempts > 1 }

// backoff is the delay before attempt n (1-based for the *next* try): exponential from Base,
// capped at Max, with ±20% jitter so concurrent jobs don't resynchronise on the limit. A
// provider-supplied retryAfter wins when it is longer.
func (p RetryPolicy) backoff(n int, retryAfter time.Duration) time.Duration {
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

// retry runs fn, retrying only rate-limit errors with backoff until the policy is exhausted or
// the context is cancelled. The final rate-limit error is wrapped with the attempt count so a
// give-up is distinguishable from a one-shot failure; non-rate-limit errors return immediately.
func retry(ctx context.Context, p RetryPolicy, fn func() error) error {
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
		ra, isRL := asRateLimit(err)
		if !isRL || n == attempts {
			break
		}
		wait := p.backoff(n, ra)
		if p.OnRetry != nil {
			p.OnRetry(n, wait, err)
		}
		if cerr := sleep(ctx, wait); cerr != nil {
			return cerr
		}
	}
	if _, isRL := asRateLimit(err); isRL && attempts > 1 {
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

// retryingImage / retryingSpeech wrap a generator so rate-limit failures back off and retry.
type retryingImage struct {
	inner  ImageGenerator
	policy RetryPolicy
}

func (r retryingImage) Generate(ctx context.Context, req ImageRequest) (ImageResult, error) {
	var res ImageResult
	err := retry(ctx, r.policy, func() error {
		var e error
		res, e = r.inner.Generate(ctx, req)
		return e
	})
	return res, err
}

type retryingSpeech struct {
	inner  SpeechGenerator
	policy RetryPolicy
}

func (r retryingSpeech) Generate(ctx context.Context, req SpeechRequest) (SpeechResult, error) {
	var res SpeechResult
	err := retry(ctx, r.policy, func() error {
		var e error
		res, e = r.inner.Generate(ctx, req)
		return e
	})
	return res, err
}

// withImageRetry / withSpeechRetry wrap a generator when the policy retries, else return it as-is.
func withImageRetry(g ImageGenerator, p RetryPolicy) ImageGenerator {
	if !p.enabled() {
		return g
	}
	return retryingImage{inner: g, policy: p}
}

func withSpeechRetry(g SpeechGenerator, p RetryPolicy) SpeechGenerator {
	if !p.enabled() {
		return g
	}
	return retryingSpeech{inner: g, policy: p}
}
