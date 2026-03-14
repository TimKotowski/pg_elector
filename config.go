package pg_elector

import "time"

type Config struct {
	// ElectionClock configures timing for leader election.

	// Timing Constraints:
	// 	LeadRetryPeriod <  LeaderDeadline < ElectionInterval
	// If none of these are met, defaults will be chosen.

	// ElectionInterval should be at least 2x the LeaderDeadline. LeaderDeadline should allow for multiple
	// LeadRetryPeriod attempts, at least 3x to 5x.

	// ElectionClock The leader must retry faster than its deadline.
	// And followers must wait longer than the leaders' deadline before attempting to force acquire leadership.
	// Otherwise, followers will compete for leadership while the current didn't have a full life cycle to continue leadership.

	// ElectionInterval will always run no matter what, but logical clock is used in code to ensure a leader has the right
	// leadership lifecycle given before election interval takes place, to ensure a more fair leadership.
	ElectionClock ElectionClock

	// Identifies which leadership group this node is competing in.
	// Defaults to "default".
	Name string

	// ReleaseOnCancel set to true if on cancel of context, the leaderships lock should be released immediately.
	// You must ensure though, that all code actions are handled before wanting to cancel leadership immediately.
	// Once released, the elector process will gracefully shut down.

	// ReleaseOnCancel set to false, if on cancel of context. Will let the leadership expire naturally before gracefully shutting down.

	// Defaults to false.
	ReleaseOnCancel bool
}

type ElectionClock struct {
	// LeaderDeadline is the duration the leader will keep retrying to refresh leadership before giving up.
	// If the leader can't successfully renew within the time window, it stops being a leader.

	// Defaults to 10 seconds
	LeaderDeadline time.Duration
	// LeadRetryPeriod is the duration at which the current leader retries renewing leadership (heartbeat).

	// Defaults to 2 seconds.
	LeadRetryPeriod time.Duration
	// ElectionInterval is the duration a non leader (follower) will wait before trying to force-acquire leadership.
	// This will always run on each node, no matter what. To try and acquire leadership.

	// Defaults to 15 seconds.
	ElectionInterval time.Duration
	// ElectionJitterInterval is base duration to apply random jitter to followers before force-acquiring leadership.
	// Even though postgres MVCC will solve concurrent forced acquiring concurrent leadership attempt so only one node wins leadership.
	// Jitter is used to reduce any unnecessary load on the database.
	// Without jitter, all followers would attempt to force-acquire leadership at roughly the same time, causing a thundering herd of competing transactions.
	// Jitter staggers these attempts so that typically one follower wins cleanly without the others competing.
	//
	// Defaults to 300 milliseconds.
	ElectionJitterInterval time.Duration
}

type ConfigFunc func(c *Config)

func NewConfig(opts ...ConfigFunc) *Config {
	var conf *Config
	for _, opt := range opts {
		opt(conf)
	}

	return conf
}

func WithElectionClock(clock ElectionClock) ConfigFunc {
	return func(c *Config) {
		c.ElectionClock = clock
	}
}

func WithName(name string) ConfigFunc {
	return func(c *Config) {
		c.Name = name
	}
}

func WithReleaseOnCancel(releaseOnCancel bool) ConfigFunc {
	return func(c *Config) {
		c.ReleaseOnCancel = releaseOnCancel
	}
}
