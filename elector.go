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
	nodeId          string
	ctx             context.Context
	driver          driver.Driver
	clock           Clock
	logger          *slog.Logger
	maxErrAttempts  int
	contextWatcher  *ContextWatcher
	electionClock   ElectionClock
	releaseOnCancel bool
	name            string
	leaderCallback  *LeaderCallback
	leader          *driver.Leader
	mutex           sync.Mutex
	stop            chan struct{}
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
		logger.Info("hitting handler")
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
		ctx:             ctx,
		nodeId:          nodeId,
		driver:          drv,
		clock:           NewClock(),
		electionClock:   config.ElectionClock,
		releaseOnCancel: config.ReleaseOnCancel,
		name:            config.Name,
		leaderCallback:  config.LeaderCallback,
		logger:          logger,
		mutex:           sync.Mutex{},
		maxErrAttempts:  5,
		stop:            make(chan struct{}),
	}

	if config.ReleaseOnCancel {
		elector.contextWatcher = NewContextWatcher(handler, ctx)
	}

	return elector, nil
}

func (e *Elector) Start(ctx context.Context) {
	if e.releaseOnCancel {
		e.contextWatcher.Watch()
	}

	go e.runElectorLoop(ctx)
}

func (e *Elector) Stop() {
	<-e.stop
}

func (e *Elector) runElectorLoop(ctx context.Context) {
	var errorCount int
	electionTimer := time.NewTimer(0)

	defer close(e.stop)
	for {
		leader, err := e.driver.GetQuerier().AcquireLeadership(ctx, driver.AcquireLeadershipParams{
			BasePrams: driver.BasePrams{
				Name:     e.name,
				LeaderId: e.nodeId,
			},
			LeaseDurationSeconds: e.electionClock.LeaseDuration.Seconds(),
		})
		if err != nil {
			errorCount++
			if errorCount >= e.maxErrAttempts {
				return
			}
			WaitCancelableBlocking(ctx, errorCount, JitterMin, JitterMax)
			continue
		} else {
			errorCount = 0
		}

		if leader != nil {
			e.mutex.Lock()
			e.leader = leader
			e.mutex.Unlock()

			if err := e.maintainLeadership(ctx); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				e.logger.ErrorContext(ctx, "Failed to maintain leadership", "error", err)
			}
		}

		// For elections, use a fixed base + jitter
		jitter := applyJitter(e.electionClock.ElectionJitterInterval, JitterMin, JitterMax)
		electionTimer.Reset(e.electionClock.ElectionInterval + jitter)

		select {
		case <-ctx.Done():
			return
		case <-electionTimer.C:
		}
	}
}

func (e *Elector) maintainLeadership(ctx context.Context) error {
	lCtx, leaderCancel := context.WithCancel(ctx)
	go e.leaderCallback.OnStartedLeading(lCtx, &ElectedLeader{
		LeaderID: e.nodeId,
		Name:     e.name,
		Term:     e.leader.Term,
	})
	go e.leaderCallback.OnNewLeader(e.nodeId)

	stepdownLeadership := func() error {
		leaderCancel()
		err := e.driver.GetQuerier().ResignLeadership(ctx, driver.BasePrams{
			Name:     e.name,
			LeaderId: e.nodeId,
		})

		return err
	}

	var errorCount int
	renewalTimer := time.NewTicker(e.electionClock.LeaderRetryPeriod)
	deadlineTimer := time.NewTimer(e.electionClock.LeaderDeadline)

	defer e.leaderCallback.OnStoppedLeading()
	for {
		select {
		case <-renewalTimer.C:
			err := e.tryRenewLeadership(ctx, func() bool {
				hasRenewDeadlineExpired := e.leader.RenewedAt.Add(e.electionClock.LeaderDeadline).Before(e.clock.NowUTC())
				if hasRenewDeadlineExpired {
					return true
				}
				return false
			})

			if err != nil {
				errorCount++
				if errors.Is(err, ErrRevokedLeader) || errors.Is(err, ErrDeadlineReached) {
					resignErr := stepdownLeadership()
					if resignErr != nil {
						e.logger.ErrorContext(ctx, "Failed to Resign Leadership", "error", err)
					}
					return err
				}

				if errorCount >= e.maxErrAttempts {
					resignErr := stepdownLeadership()
					if resignErr != nil {
						e.logger.ErrorContext(ctx, "Failed to Resign Leadership", "error", err)
					}
					return err
				}
			} else {
				deadlineTimer.Reset(e.electionClock.LeaderDeadline)
				errorCount = 0
			}

		case <-deadlineTimer.C:
			err := stepdownLeadership()
			if err != nil {
				e.logger.ErrorContext(ctx, "Failed to Resign Leadership", "error", err)
			}
			return nil

		case <-ctx.Done():
			// First: Send cancel signal on the leader.
			leaderCancel()
			// Second: Release leadership immediately, so followers can fore-acquire.
			if e.releaseOnCancel {
				<-e.contextWatcher.Release()
			}
			return ctx.Err()
		}
	}
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

	ctxTimeout, cancel := context.WithTimeout(ctx, e.electionClock.LeaderDeadline)
	acquiredLeaderRenewal, err := e.driver.GetQuerier().LeaderRenewal(ctxTimeout, driver.LeaderRenewalParams{
		BasePrams: driver.BasePrams{
			Name:     e.name,
			LeaderId: e.nodeId,
		},
		LeseDuration: e.electionClock.LeaseDuration.Seconds(),
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
