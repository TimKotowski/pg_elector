package pg_elector

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/TimKotowski/pg_elector/driver"
)

var ErrRevokedLeader = errors.New("leadership was revoked")
var ErrDeadlineReached = errors.New("leader deadline duration reached, unable to successfully renew lease")

type Elector struct {
	ctx                   context.Context
	nodeId                string
	driver                driver.Driver
	clock                 Clock
	config                *Config
	logger                *slog.Logger
	mutex                 sync.Mutex
	maxRenewalErrAttempts int

	contextWatcher *ContextWatcher

	leaderCallbackContextWatcher *ContextWatcher

	leader *driver.Leader
}
type electorConfig struct {
	electionClock ElectionClock

	releaseOnCancel bool

	name string

	leaderCallback LeaderCallback
}

type ElectedLeader struct {
	Name     string
	LeaderID string
	Term     int64
}

func NewLeaderElector(ctx context.Context, drv driver.Driver, config *Config) (*Elector, error) {
	nodeId, err := getNodeId()
	if err != nil {
		return nil, err
	}

	if drv == nil {
		return nil, errors.New("database driver was uninitialized")
	}

	if config == nil {
		return nil, errors.New("config cannot be nil")
	}

	config = config.WithDefaults()

	if err := config.validate(); err != nil {
		return nil, err
	}

	logger := config.Logger
	logger = logger.With(
		slog.String("component", "elector"),
		slog.String("nodeId", nodeId),
		slog.String("election", config.Name),
	)

	handler := func() {
		if err := drv.GetQuerier().ResignLeadership(context.Background(), driver.BasePrams{
			Name:     config.Name,
			LeaderId: nodeId,
		}); err != nil {
			logger.Warn("release on cancel context was called, where best-effort leader resign failed",
				"error", err.Error(),
			)
		}
	}

	elector := &Elector{
		ctx:                   ctx,
		nodeId:                nodeId,
		driver:                drv,
		clock:                 NewClock(),
		config:                config,
		logger:                logger,
		mutex:                 sync.Mutex{},
		maxRenewalErrAttempts: 5,
		leader:                nil,
	}

	if config.ReleaseOnCancel {
		elector.contextWatcher = NewContextWatcher(handler, ctx)
	}

	return elector, nil
}

func (e *Elector) Start(ctx context.Context) error {
	if e.config.ReleaseOnCancel {
		e.contextWatcher.Watch()
	}
	var attempts int
	electionTimer := time.NewTimer(0)
	for {
		leader, err := e.driver.GetQuerier().AcquireLeadership(context.Background(), driver.AcquireLeadershipParams{
			BasePrams: driver.BasePrams{
				Name:     e.config.Name,
				LeaderId: e.nodeId,
			},
			LeaseDurationSeconds: e.config.ElectionClock.LeaseDuration.Seconds(),
		})
		if err != nil {
			attempts++
			WaitBlocking(ctx, attempts)
			continue
		}

		if leader != nil {
			e.mutex.Lock()
			e.leader = leader
			e.mutex.Unlock()

			if err := e.runBlockingLeadershipLoop(ctx); errors.Is(err, context.Canceled) {
				return err
			}
		}

		attempts = 0
		jitter := JitterDuration(e.config.ElectionClock.ElectionJitterInterval)
		electionTimer.Reset(e.config.ElectionClock.ElectionInterval + jitter)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-electionTimer.C:
		}
	}
}

func (e *Elector) isLeader() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	if e.leader == nil {
		return false
	}

	now := e.clock.NowUTC()
	if e.leader.RenewedAt.Add(e.config.ElectionClock.LeaderDeadline).Before(now) {
		return false
	}

	return true
}

func (e *Elector) isFollower() bool {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	return e.leader == nil
}

func (e *Elector) runBlockingLeadershipLoop(ctx context.Context) error {
	renewalTimer := time.NewTicker(e.config.ElectionClock.LeaderRetryPeriod)
	deadlineTimer := time.NewTicker(e.config.ElectionClock.LeaderDeadline)

	lCtx, leaderCancel := context.WithCancel(ctx)
	go e.config.LeaderCallback.OnStartedLeading(lCtx, &ElectedLeader{
		LeaderID: e.nodeId,
		Name:     e.config.Name,
		Term:     e.leader.Term,
	})
	go e.config.LeaderCallback.OnNewLeader(e.nodeId)

	e.leaderCallbackContextWatcher = NewContextWatcher(func() { e.config.LeaderCallback.OnStoppedLeading() }, lCtx)
	go e.leaderCallbackContextWatcher.Watch()

	var attempts int
	for {
		select {
		case <-renewalTimer.C:
			attempts++
			err := e.tryRenewLeadership(ctx, func() bool {
				now := e.clock.NowUTC()
				hasRenewDeadlineExpired := e.leader.RenewedAt.Add(e.config.ElectionClock.LeaderDeadline).Before(now)
				if hasRenewDeadlineExpired {
					return true
				}
				return false
			})

			if err != nil {
				if errors.Is(err, ErrRevokedLeader) || errors.Is(err, ErrDeadlineReached) {
					e.revokeInternalStateLeadership()
					leaderCancel()
					<-e.leaderCallbackContextWatcher.Release()
					_ = e.driver.GetQuerier().ResignLeadership(ctx, driver.BasePrams{
						Name:     e.config.Name,
						LeaderId: e.nodeId,
					})
					return err
				}

				if attempts > e.maxRenewalErrAttempts {
					e.revokeInternalStateLeadership()
					leaderCancel()
					<-e.leaderCallbackContextWatcher.Release()
					_ = e.driver.GetQuerier().ResignLeadership(ctx, driver.BasePrams{
						Name:     e.config.Name,
						LeaderId: e.nodeId,
					})
					return err
				}
			} else {
				attempts = 0
			}

		case <-deadlineTimer.C:
			e.revokeInternalStateLeadership()
			leaderCancel()
			<-e.leaderCallbackContextWatcher.Release()

		case <-ctx.Done():
			// First: Release any work that may be happening on the leader.
			e.revokeInternalStateLeadership()
			leaderCancel()
			<-e.leaderCallbackContextWatcher.Release()

			// Second: Release leadership immediately, so followers can fore-acquire.
			if e.config.ReleaseOnCancel {
				<-e.contextWatcher.Release()
			}
			return ctx.Err()
		}
	}
}

func (e *Elector) revokeInternalStateLeadership() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.leader = nil
}

func (e *Elector) tryRenewLeadership(ctx context.Context, hasLeaderDeadlineBeenReached func() bool) error {
	// With timer for LeaderDeadlineTimer and RenewTimer the goroutine scheduling order can be non-deterministic.
	// The Go runtime does not guarantee if renew timer or deadline timer if both concurrently firing timers, will have its goroutine
	// scheduled first, which could allow the renewal timer to execute before the deadline timer, when both have elapsed.
	// And depending on the LeaseDuration that was set (non default used), could habe a rather tight TTL,
	// that could cause leadership split-brain scenarios.
	if hasLeaderDeadlineBeenReached() {
		return ErrDeadlineReached
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, e.config.ElectionClock.LeaderDeadline)
	acquiredLeaderRenewal, err := e.driver.GetQuerier().LeaderRenewal(ctxTimeout, driver.LeaderRenewalParams{
		BasePrams: driver.BasePrams{
			Name:     e.config.Name,
			LeaderId: e.nodeId,
		},
		LeseDuration: e.config.ElectionClock.LeaseDuration.Seconds(),
	})
	cancel()

	if err == nil && acquiredLeaderRenewal == nil {
		return ErrRevokedLeader
	}

	if err != nil {
		return err
	}

	e.mutex.Lock()
	e.leader = acquiredLeaderRenewal
	e.mutex.Unlock()

	return nil
}
