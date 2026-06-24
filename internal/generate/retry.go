package generate

import (
	"context"

	"github.com/jmylchreest/colophon/internal/retry"
)

// RetryPolicy and DefaultRetryPolicy re-export the shared retry policy so build/audio code can
// configure provider backoff (logging hook, --no-backoff) without importing internal/retry too.
type RetryPolicy = retry.Policy

// DefaultRetryPolicy is the standard provider backoff (~6 attempts, exp 2s→45s).
func DefaultRetryPolicy() RetryPolicy { return retry.Default() }

// retryingImage / retryingSpeech wrap a generator so rate-limit and transient failures back off
// and retry. Image and speech generation are idempotent (cached by content hash), so they retry
// every retryable kind.
type retryingImage struct {
	inner  ImageGenerator
	policy RetryPolicy
}

func (r retryingImage) Generate(ctx context.Context, req ImageRequest) (ImageResult, error) {
	var res ImageResult
	err := retry.Do(ctx, r.policy, func() error {
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
	err := retry.Do(ctx, r.policy, func() error {
		var e error
		res, e = r.inner.Generate(ctx, req)
		return e
	})
	return res, err
}

// withImageRetry / withSpeechRetry wrap a generator when the policy retries, else return it as-is.
func withImageRetry(g ImageGenerator, p RetryPolicy) ImageGenerator {
	if !p.Enabled() {
		return g
	}
	return retryingImage{inner: g, policy: p}
}

func withSpeechRetry(g SpeechGenerator, p RetryPolicy) SpeechGenerator {
	if !p.Enabled() {
		return g
	}
	return retryingSpeech{inner: g, policy: p}
}
