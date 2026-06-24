package generate

import (
	"testing"

	"github.com/jmylchreest/colophon/internal/retry"
)

func TestMinimaxStatusErrorClassifies(t *testing.T) {
	for _, code := range []int{1002, 1039} {
		if _, ok := retry.AsRetryable(minimaxStatusError(code, "x")); !ok {
			t.Errorf("MiniMax code %d should be a rate limit", code)
		}
	}
	for _, code := range []int{1004, 2013, 1000} {
		if _, ok := retry.AsRetryable(minimaxStatusError(code, "x")); ok {
			t.Errorf("MiniMax code %d should NOT be retryable", code)
		}
	}
}
