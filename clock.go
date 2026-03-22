package pg_elector

import "time"

type clock struct{}

// NewClock allow time mocking.
func NewClock() Clock {
	return &clock{}
}

type Clock interface {
	NowUTC() time.Time
}

func (c *clock) NowUTC() time.Time {
	return time.Now().UTC()
}
