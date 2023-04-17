package wait

import (
	"context"
	"time"
)

// WithJitter waits for the specified interval, plus or minus a random fraction of the interval
// specified by jitter. If jitter is outside the range [0,1) it is ignored.
// The function returns after the adjusted interval or if the context is cancelled, in which case
// it returns the cancellation error.
func WithJitter(ctx context.Context, interval time.Duration, jitter float64) error {
	if interval <= 0 {
		return nil
	}

	interval = JitterDuration(interval, jitter)

	t := time.NewTimer(interval)
	defer t.Stop()

	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
