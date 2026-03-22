package pg_elector

import (
	"cmp"
	"context"
	"log/slog"
	"os"
	"time"
)

type Config struct {
	LeaderCallback LeaderCallback
	// ElectionClock configures timing for leader election.

	// Timing Constraints:
	// 	LeaderRetryPeriod <  LeaderDeadline < ElectionInterval
	// If none of these are met, defaults will be chosen.

	// ElectionInterval should be at least 2x the LeaderDeadline. LeaderDeadline should allow for multiple
	// LeaderRetryPeriod attempts, at least 3x to 5x.

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

	// Defaults to true.
	ReleaseOnCancel bool

	Logger *slog.Logger
}

type ElectionClock struct {
	// LeaderDeadline is the duration the leader will keep retrying to refresh leadership before giving up.
	// If the leader can't successfully renew within the time window, it stops being a leader.

	// Defaults to 10 seconds
	LeaderDeadline time.Duration
	// LeaderRetryPeriod is the duration at which the current leader retries renewing leadership (heartbeat).

	// Defaults to 2 seconds.
	LeaderRetryPeriod time.Duration
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
	// LeaseDurationSeconds is the duration at which the lease will be held by leader, if not renewed.
	// For a tighter re-election, then this should be set.
	// If a leader was revoked due to being passed the LeaderDeadline or from a release, followers must wait for
	// the full LeaseDurationSeconds period before any force-acquiring leadership can be successful. This can sometimes be
	// unacceptable. So setting a tighter LeaseDurationSeconds time makes sense in case where this matters.

	// Defaults to ElectionInterval + LeaderDeadline
	LeaseDuration time.Duration
}

type LeaderCallback struct {
	OnStartedLeading func(ctx context.Context)
	OnStoppedLeading func()
	OnNewLeader      func(nodeId string)
}

type ConfigFunc func(c *Config)

func (c *Config) WithDefaults() *Config {
	if c == nil {
		c = &Config{}
	}

	if c.Logger == nil {
		c.Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelWarn,
		}))
	}

	return &Config{
		Name:           cmp.Or(c.Name, "default"),
		Logger:         c.Logger,
		LeaderCallback: c.LeaderCallback,
		ElectionClock: ElectionClock{
			ElectionInterval:       cmp.Or(c.ElectionClock.ElectionInterval, time.Second*15),
			LeaderDeadline:         cmp.Or(c.ElectionClock.LeaderDeadline, time.Second*10),
			LeaderRetryPeriod:      cmp.Or(c.ElectionClock.LeaderRetryPeriod, time.Second*2),
			ElectionJitterInterval: cmp.Or(c.ElectionClock.ElectionJitterInterval, time.Millisecond*300),
			LeaseDuration:          cmp.Or(c.ElectionClock.LeaseDuration, c.ElectionClock.ElectionInterval+c.ElectionClock.LeaderDeadline),
		},
		ReleaseOnCancel: c.ReleaseOnCancel,
	}
}

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
