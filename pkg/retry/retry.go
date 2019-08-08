package retry

import (
	"context"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/gravitational/trace"
)

// WithInterval retries the specified operation fn using the specified backoff interval.
// classify specifies the error classifier that can create circuit-breakers for
// specific error conditions. classify should return backoff.PermanentError if the error
// should not be retried and returned directly.
// Returns nil on success or the last received error upon exhausting the interval.
func WithInterval(ctx context.Context, interval backoff.BackOff, fn func() error) error {
	b := backoff.WithContext(interval, ctx)
	err := backoff.Retry(func() (err error) {
		err = fn()
		return err
	}, b)

	switch errOrig := trace.Unwrap(err).(type) {
	case *trace.RetryError:
		// TODO: fix trace.Retry.OrigError to return the original error
		err = errOrig.Err
	}
	if err != nil {
		return trace.Wrap(err)
	}
	return nil
}

// NewUnlimitedExponentialBackOff returns a backoff interval without time restriction
func NewUnlimitedExponentialBackOff() *backoff.ExponentialBackOff {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 0
	return b
}

// NewExponentialBackOff creates a new backoff interval with the specified timeout
func NewExponentialBackOff(timeout time.Duration) backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = timeout
	return b
}
