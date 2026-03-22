package pg_elector

import (
	"context"
	"math"
	"time"
)

var (
	RetryMax     = 5
	RetryWaitMin = time.Second * 2
	RetryWaitMax = time.Second * 30
)

type retry struct {
	ctx  context.Context
	time *time.Timer
}

func exponentialBackoffWithJitter(attempts int) time.Duration {
	mult := math.Pow(2, float64(attempts))
	wait := time.Duration(float64(RetryWaitMin) * mult)
	if wait > RetryWaitMax {
		wait = RetryWaitMax
	}
	wait = wait + JitterDuration(wait)

	if wait > RetryWaitMax {
		wait = RetryWaitMax
	}

	return wait
}

func WaitBlocking(ctx context.Context, attempts int) {
	wait := time.NewTimer(exponentialBackoffWithJitter(attempts))

	select {
	case <-wait.C:
	case <-ctx.Done():
	}
}
