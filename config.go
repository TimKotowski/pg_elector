package pg_elector

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"os"
	"time"
)

type LeaderTask = func(ctx context.Context, leader *ElectedLeader)

type Config struct {
	// LeaderCallback configures the callbacks of tasks for leader.
	LeaderCallback *LeaderCallback
	// ElectionClock configures clocks for leader election and leader life cycle.

	// Clock Constraints:
	// 		LeaderRetryPeriod < LeaderDeadline < ElectionInterval < LeaseDuration

	// ElectionInterval should be at least 1-2x the LeaderDeadline.
	// LeaderDeadline should allow for multiple LeaderRetryPeriod attempts, at least 3x to 5x. The leader must retry faster than its deadline.
	// LeaseDuration should be 1x or more of ElectionInterval for proper leader life cycles.
	// Tighter LeaserDuration is allowed, if needed.
	// Be aware, if ReleaseOnCancel is set to false, LeaseDuration will be held by released leader, till LeaserDuration has elapsed.
	// This would be a potentially long duration of no leader activity.
	ElectionClock ElectionClock

	// Identifies which leadership group this node is competing in.
	// Defaults to "default".
	Name string

	// ReleaseOnCancel set to true if on cancel of context, the leaderships lease should be released immediately.
	// You must ensure though, that all code actions are handled before wanting to cancel leadership immediately.

	// ReleaseOnCancel set to false, if on cancel of context. Will let the leadership expire naturally.
	// Be aware, if ReleaseOnCancel is set to default false, LeaseDuration will be held by released leader, till LeaserDuration has elapsed.
	// This would be a potentially long duration of no leader activity.

	// Defaults to false.
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

// LeaderCallback configures the callbacks of tasks for leader.
type LeaderCallback struct {
	// OnStartedLeading configures the callback on leader to allow tasks to start.
	// A context and leader object is provided.

	// Context allows to side step stale leaders more safely, and allow the stale leader to clean up work, or notify work to stop.

	// Leader object is provided with info about the elected leader as a token fencing mechanism.
	// This goes a long way toward a strong single leader safety. It can prevent a stale acting leader from causing any damage.
	// Every time a new leader is elected, the leader includes a monotonically increasing term.
	OnStartedLeading LeaderTask

	OnStoppedLeading func()

	OnNewLeader func(nodeId string)
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
	slog.SetDefault(c.Logger)

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

func (c *Config) validate() error {
	if c.LeaderCallback == nil {
		return errors.New("leader callbacks cannot be nil")
	}

	if c.LeaderCallback.OnStartedLeading == nil {
		return errors.New("leader callbacks cannot be nil")
	}
	if c.LeaderCallback.OnStartedLeading == nil {
		return errors.New("leader callbacks cannot be nil")
	}
	if c.LeaderCallback.OnNewLeader == nil {
		return errors.New("leader callbacks cannot be nil")
	}

	validElectoralClock := c.ElectionClock.LeaderRetryPeriod < c.ElectionClock.LeaderDeadline &&
		c.ElectionClock.LeaderDeadline < c.ElectionClock.ElectionInterval &&
		c.ElectionClock.ElectionInterval < c.ElectionClock.LeaseDuration

	if !validElectoralClock {
		return errors.New("incorrect electoral clock configuration")
	}

	return nil
}
