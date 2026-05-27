package bot

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/thehappydinoa/signal-go/internal/web"
)

// RateLimitRetryOptions configures [RateLimitRetryMiddleware].
type RateLimitRetryOptions struct {
	// MaxRetries is how many retry attempts to make after a 429 error.
	// Default: 1.
	MaxRetries int
	// DefaultDelay is used when the 429 does not include Retry-After.
	// Default: 2s.
	DefaultDelay time.Duration
}

// RateLimitRetryMiddleware retries handlers on HTTP 429 errors.
//
// The middleware uses Retry-After when present and falls back to
// [RateLimitRetryOptions.DefaultDelay]. It returns the last error after
// retries are exhausted or if the context is canceled.
func RateLimitRetryMiddleware(opts RateLimitRetryOptions) MiddlewareFunc {
	maxRetries := opts.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}
	defaultDelay := opts.DefaultDelay
	if defaultDelay <= 0 {
		defaultDelay = 2 * time.Second
	}

	return func(next HandlerFunc) HandlerFunc {
		return func(ctx context.Context, m *Message, args []string) error {
			var lastErr error
			for attempt := 0; attempt <= maxRetries; attempt++ {
				lastErr = next(ctx, m, args)
				if lastErr == nil {
					return nil
				}
				retryAfter, ok := rateLimitRetryAfter(lastErr)
				if !ok || attempt == maxRetries {
					return lastErr
				}
				if retryAfter <= 0 {
					retryAfter = defaultDelay
				}

				t := time.NewTimer(retryAfter)
				select {
				case <-ctx.Done():
					t.Stop()
					return ctx.Err()
				case <-t.C:
				}
			}
			return lastErr
		}
	}
}

func rateLimitRetryAfter(err error) (time.Duration, bool) {
	var werr *web.Error
	if errors.As(err, &werr) {
		if werr.StatusCode == 429 {
			return werr.RetryAfter, true
		}
		return 0, false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "http 429") || strings.Contains(msg, "too many requests") {
		return 0, true
	}
	return 0, false
}
