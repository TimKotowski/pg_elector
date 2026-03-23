package pg_elector

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestJitterDuration(t *testing.T) {
	t.Run("jitter output is always within min and max range of base", func(t *testing.T) {
		baseJitter := time.Millisecond * 300
		expectedMinJitter := calculateExpectedJitter(t, baseJitter, JitterMin)
		expectedMaxJitter := calculateExpectedJitter(t, baseJitter, JitterMax)
		jitter := applyJitter(baseJitter, JitterMin, JitterMax)

		assert.LessOrEqual(t, jitter, expectedMaxJitter)
		assert.GreaterOrEqual(t, jitter, expectedMinJitter)
	})

	t.Run("jitter default scaling range is used when min > max", func(t *testing.T) {
		baseJitter := time.Millisecond * 300
		expectedMinJitter := calculateExpectedJitter(t, baseJitter, JitterMin)
		expectedMaxJitter := calculateExpectedJitter(t, baseJitter, JitterMax)
		jitter := applyJitter(baseJitter, 1.0, 0.6)

		assert.LessOrEqual(t, jitter, expectedMaxJitter)
		assert.GreaterOrEqual(t, jitter, expectedMinJitter)
	})
}

func calculateExpectedJitter(t *testing.T, base time.Duration, scale float64) time.Duration {
	t.Helper()

	return time.Duration(float64(base) * scale)
}
