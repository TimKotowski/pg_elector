package pg_elector

import (
	"math/rand/v2"
	"time"
)

// For elections a good node stagger, use 0.5 to 1.1
// [0.5-1.1]
const (
	JitterMin = 0.5
	JitterMax = 1.1
)

func applyJitter(d time.Duration, min, max float64) time.Duration {
	if min > max {
		min, max = max, min
	}

	jitter := min + rand.Float64()*(max-min)
	return time.Duration(float64(d) * jitter)
}
