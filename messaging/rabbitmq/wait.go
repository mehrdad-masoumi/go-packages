package rabbitmq

import (
	"context"
	"time"
)

// Wait blocks until d elapses or ctx is cancelled.
// Reconnect and consume retry loops must use this instead of time.Sleep.
func Wait(ctx context.Context, d time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
