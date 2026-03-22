package pg_elector

import (
	"math/rand/v2"
	"time"
)

// Todo fix to add own scale.
func JitterDuration(d time.Duration) time.Duration {
	// [0.5-1.1]
	jitter := 0.5 + rand.Float64()*0.6
	return time.Duration(float64(d) * jitter)
}
