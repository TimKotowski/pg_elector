package pg_elector

import (
	"context"
	"math"
	"time"
)

var (
	MinRetryDelay = time.Second * 2
	MaxRetryDelay = time.Second * 30
)

func exponentialBackoffWithJitter(attempts int, jitterMin, jitterMax float64) time.Duration {
	mult := math.Pow(2, float64(attempts))
	wait := time.Duration(float64(MinRetryDelay) * mult)
	if wait > MaxRetryDelay {
		wait = MaxRetryDelay
	}
	wait = applyJitter(wait, jitterMin, jitterMax)

	if wait > MaxRetryDelay {
		wait = MaxRetryDelay
	}

	return wait
}

func WaitBlocking(ctx context.Context, attempts int, jitterMin, jitterMax float64) {
	wait := time.NewTimer(exponentialBackoffWithJitter(attempts, jitterMin, jitterMax))

	select {
	case <-wait.C:
	case <-ctx.Done():
	}
}
