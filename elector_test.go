package pg_elector

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/TimKotowski/pg_elector/driver"
	mockDriver2 "github.com/TimKotowski/pg_elector/mocks"
)

func TestSingleNodeElector(t *testing.T) {
	t.Parallel()

	t.Run("when ReleaseOnCancel is true, leader node revoked leadership immediately on context cancel", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		d := mockDriver2.NewMockDriver(ctrl)
		querier := mockDriver2.NewMockQuerier(ctrl)

		startedLeading := make(chan struct{}, 1)
		onStoppedLeader := make(chan struct{}, 1)
		ctx, cancel := context.WithCancel(context.Background())
		elector, err := NewLeaderElector(ctx, d, &Config{
			ElectionClock: ElectionClock{
				LeaderDeadline:         time.Second * 2,
				LeaderRetryPeriod:      time.Millisecond * 100,
				ElectionInterval:       time.Second * 3,
				ElectionJitterInterval: time.Millisecond * 80,
			},
			LeaderCallback: &LeaderCallback{
				OnStartedLeading: func(ctx context.Context, leader *ElectedLeader) {
					startedLeading <- struct{}{}
				},
				OnStoppedLeading: func() {
					onStoppedLeader <- struct{}{}
				},
				OnNewLeader: func(nodeId string) {},
			},
			Name:            "pg_elector",
			ReleaseOnCancel: true,
		})
		assert.NoError(t, err)

		d.EXPECT().GetQuerier().AnyTimes().Return(querier)
		querier.EXPECT().AcquireLeadership(gomock.Any(), gomock.Any()).Return(&driver.Leader{
			ElectedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
			RenewedAt: time.Now().UTC(),
			Name:      "pg_elector",
			LeaderID:  "leader-001",
		}, nil)
		querier.EXPECT().LeaderRenewal(gomock.Any(), gomock.Any()).AnyTimes().Return(&driver.Leader{
			ElectedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
			RenewedAt: time.Now().UTC(),
			Name:      "pg_elector",
			LeaderID:  "leader-001",
		}, nil)
		querier.EXPECT().ResignLeadership(gomock.Any(), gomock.Any()).Times(1).Return(nil)

		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			err := elector.Start(ctx)
			assert.ErrorIs(t, err, context.Canceled)
		}(wg)

		<-startedLeading
		assert.True(t, elector.isLeader())

		wait := time.NewTimer(elector.electionClock.LeaderRetryPeriod)
		select {
		case <-wait.C:
		}
		cancel()
		<-onStoppedLeader

		wg.Wait()
		assert.False(t, elector.isLeader())
	})

	t.Run("when ReleaseOnCancel is false, leadership is naturally released by waiting for lease duration to expire", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		d := mockDriver2.NewMockDriver(ctrl)
		querier := mockDriver2.NewMockQuerier(ctrl)

		startedLeading := make(chan struct{}, 1)
		onStoppedLeader := make(chan struct{}, 1)
		ctx, cancel := context.WithCancel(context.Background())
		elector, err := NewLeaderElector(ctx, d, &Config{
			ElectionClock: ElectionClock{
				LeaderDeadline:         time.Second * 1,
				LeaderRetryPeriod:      time.Millisecond * 100,
				ElectionInterval:       time.Second * 2,
				ElectionJitterInterval: time.Millisecond * 80,
			},
			LeaderCallback: &LeaderCallback{
				OnStartedLeading: func(ctx context.Context, leader *ElectedLeader) {
					startedLeading <- struct{}{}
				},
				OnStoppedLeading: func() {
					onStoppedLeader <- struct{}{}
				},
				OnNewLeader: func(nodeId string) {},
			},
			Name:            "pg_elector",
			ReleaseOnCancel: false,
		})
		assert.NoError(t, err)

		d.EXPECT().GetQuerier().AnyTimes().Return(querier)
		querier.EXPECT().AcquireLeadership(gomock.Any(), gomock.Any()).Return(&driver.Leader{
			ElectedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
			RenewedAt: time.Now().UTC(),
			Name:      "pg_elector",
			LeaderID:  "leader-001",
		}, nil)
		querier.EXPECT().LeaderRenewal(gomock.Any(), gomock.Any()).AnyTimes().Return(&driver.Leader{
			ElectedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
			RenewedAt: time.Now().UTC(),
			Name:      "pg_elector",
			LeaderID:  "leader-001",
		}, nil)
		querier.EXPECT().ResignLeadership(gomock.Any(), gomock.Any()).Times(0).Return(nil)

		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			err := elector.Start(ctx)
			assert.ErrorIs(t, err, context.Canceled)
		}(wg)

		<-startedLeading
		assert.True(t, elector.isLeader())

		wait := time.NewTimer(elector.electionClock.LeaderRetryPeriod)
		select {
		case <-wait.C:
		}
		cancel()
		<-onStoppedLeader

		wg.Wait()
		assert.False(t, elector.isLeader())
	})

	t.Run("successful renewals keep leader beyond initial deadline window", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		d := mockDriver2.NewMockDriver(ctrl)
		querier := mockDriver2.NewMockQuerier(ctrl)

		startedLeading := make(chan struct{}, 1)
		onStoppedLeader := make(chan struct{}, 1)
		ctx, cancel := context.WithCancel(context.Background())
		elector, err := NewLeaderElector(ctx, d, &Config{
			ElectionClock: ElectionClock{
				LeaderDeadline:         time.Second * 1,
				LeaderRetryPeriod:      time.Millisecond * 100,
				ElectionInterval:       time.Second * 2,
				ElectionJitterInterval: time.Millisecond * 80,
			},
			LeaderCallback: &LeaderCallback{
				OnStartedLeading: func(ctx context.Context, leader *ElectedLeader) {
					startedLeading <- struct{}{}
				},
				OnStoppedLeading: func() {
					onStoppedLeader <- struct{}{}
				},
				OnNewLeader: func(nodeId string) {},
			},

			Logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})),
			Name:            "pg_elector",
			ReleaseOnCancel: false,
		})
		mockClock := mockDriver2.NewMockClock(ctrl)
		elector.clock = mockClock
		assert.NoError(t, err)

		d.EXPECT().GetQuerier().AnyTimes().Return(querier)

		mockClock.EXPECT().NowUTC().AnyTimes().Return(time.Now().UTC())
		querier.EXPECT().AcquireLeadership(gomock.Any(), gomock.Any()).Return(&driver.Leader{
			ElectedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
			RenewedAt: time.Now().UTC(),
			Name:      "pg_elector",
			LeaderID:  "leader-001",
			Term:      1,
		}, nil)
		querier.EXPECT().LeaderRenewal(gomock.Any(), gomock.Any()).AnyTimes().Return(&driver.Leader{
			ElectedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
			RenewedAt: time.Now().UTC(),
			Name:      "pg_elector",
			LeaderID:  "leader-001",
			Term:      1,
		}, nil)
		querier.EXPECT().ResignLeadership(gomock.Any(), gomock.Any()).Times(0)

		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			err := elector.Start(ctx)
			assert.ErrorIs(t, err, context.Canceled)
		}(wg)

		<-startedLeading
		assert.True(t, elector.isLeader())

		// This test we want to make sure leadership is held for at least 2 full LeaderDeadlines so we know leader is held.
		timer := time.NewTimer(elector.electionClock.LeaderDeadline * 2)
		<-timer.C
		assert.True(t, elector.isLeader())

		cancel()
		<-onStoppedLeader
	})

	t.Run("leader resigns when renewal was revoked", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		d := mockDriver2.NewMockDriver(ctrl)
		querier := mockDriver2.NewMockQuerier(ctrl)

		startedLeading := make(chan struct{}, 1)
		onStoppedLeader := make(chan struct{}, 1)
		ctx, cancel := context.WithCancel(context.Background())
		elector, err := NewLeaderElector(ctx, d, &Config{
			ElectionClock: ElectionClock{
				LeaderDeadline:         time.Second * 1,
				LeaderRetryPeriod:      time.Millisecond * 1,
				ElectionInterval:       time.Second * 2,
				ElectionJitterInterval: time.Millisecond * 1,
			},
			LeaderCallback: &LeaderCallback{
				OnStartedLeading: func(ctx context.Context, leader *ElectedLeader) {
					startedLeading <- struct{}{}
				},
				OnStoppedLeading: func() {
					onStoppedLeader <- struct{}{}
				},
				OnNewLeader: func(nodeId string) {},
			},
			Logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})),
			Name:            "pg_elector",
			ReleaseOnCancel: false,
		})
		mockClock := mockDriver2.NewMockClock(ctrl)
		elector.clock = mockClock
		assert.NoError(t, err)

		d.EXPECT().GetQuerier().AnyTimes().Return(querier)
		mockClock.EXPECT().NowUTC().AnyTimes().Return(time.Now().UTC())
		querier.EXPECT().AcquireLeadership(gomock.Any(), gomock.Any()).Times(1).Return(&driver.Leader{
			ElectedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
			RenewedAt: time.Now().UTC(),
			Name:      "pg_elector",
			LeaderID:  "leader-001",
			Term:      1,
		}, nil)

		gomock.InOrder(
			querier.EXPECT().LeaderRenewal(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, params driver.LeaderRenewalParams) (*driver.Leader, error) {
					return &driver.Leader{
						ElectedAt: time.Now().UTC(),
						ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
						RenewedAt: time.Now().UTC(),
						Name:      "pg_elector",
						LeaderID:  "leader-001",
						Term:      1,
					}, nil
				}),
			querier.EXPECT().LeaderRenewal(gomock.Any(), gomock.Any()).Return(nil, nil),
		)
		querier.EXPECT().ResignLeadership(gomock.Any(), gomock.Any()).Times(1).Return(nil)

		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			_ = elector.Start(ctx)
		}(wg)

		<-startedLeading
		assert.True(t, elector.isLeader())

		<-onStoppedLeader
		// This test we want to make sure Resign was hit.
		timer := time.NewTimer(time.Millisecond * 100)
		<-timer.C

		assert.False(t, elector.isLeader())

		cancel()
	})

	t.Run("leader steps down when renew leadership reaches max attempts", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		d := mockDriver2.NewMockDriver(ctrl)
		querier := mockDriver2.NewMockQuerier(ctrl)
		mockClock := mockDriver2.NewMockClock(ctrl)

		startedLeading := make(chan struct{}, 1)
		onStoppedLeader := make(chan struct{}, 1)
		ctx, cancel := context.WithCancel(context.Background())
		elector, err := NewLeaderElector(ctx, d, &Config{
			ElectionClock: ElectionClock{
				LeaderDeadline:         time.Millisecond * 500,
				LeaderRetryPeriod:      time.Millisecond * 10,
				ElectionInterval:       time.Second * 1,
				ElectionJitterInterval: time.Millisecond * 10,
			},
			LeaderCallback: &LeaderCallback{
				OnStartedLeading: func(ctx context.Context, leader *ElectedLeader) {
					startedLeading <- struct{}{}
				},
				OnStoppedLeading: func() {
					onStoppedLeader <- struct{}{}
				},
				OnNewLeader: func(nodeId string) {},
			},
			Logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})),
			Name:            "pg_elector",
			ReleaseOnCancel: false,
		})
		assert.NoError(t, err)
		elector.clock = mockClock

		d.EXPECT().GetQuerier().AnyTimes().Return(querier)
		mockClock.EXPECT().NowUTC().AnyTimes().Return(time.Now().UTC())
		querier.EXPECT().AcquireLeadership(gomock.Any(), gomock.Any()).Times(1).Return(&driver.Leader{
			ElectedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
			RenewedAt: time.Now().UTC(),
			Name:      "pg_elector",
			LeaderID:  "leader-001",
			Term:      1,
		}, nil)

		querier.EXPECT().LeaderRenewal(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, errors.New("database error"))
		querier.EXPECT().ResignLeadership(gomock.Any(), gomock.Any()).Times(1).Return(nil)

		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			err := elector.Start(ctx)
			assert.Error(t, err, context.Canceled)
		}(wg)

		<-startedLeading
		assert.True(t, elector.isLeader())

		<-onStoppedLeader
		assert.True(t, elector.isFollower())

		cancel()
	})

	t.Run("leader steps down when leader deadline timer fires", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		d := mockDriver2.NewMockDriver(ctrl)
		querier := mockDriver2.NewMockQuerier(ctrl)
		mockClock := mockDriver2.NewMockClock(ctrl)

		startedLeading := make(chan struct{}, 1)
		onStoppedLeader := make(chan struct{}, 1)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		elector, err := NewLeaderElector(ctx, d, &Config{
			ElectionClock: ElectionClock{
				LeaderDeadline:         time.Millisecond * 150,
				LeaderRetryPeriod:      time.Millisecond * 120,
				ElectionInterval:       time.Second * 2,
				ElectionJitterInterval: time.Millisecond * 10,
				LeaseDuration:          time.Second * 3,
			},
			LeaderCallback: &LeaderCallback{
				OnStartedLeading: func(ctx context.Context, leader *ElectedLeader) {
					startedLeading <- struct{}{}
				},
				OnStoppedLeading: func() {
					onStoppedLeader <- struct{}{}
				},
				OnNewLeader: func(nodeId string) {},
			},
			Logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})),
			Name:            "pg_elector",
			ReleaseOnCancel: false,
		})
		assert.NoError(t, err)
		elector.clock = mockClock

		d.EXPECT().GetQuerier().AnyTimes().Return(querier)
		fixedNow := time.Now().UTC()
		mockClock.EXPECT().NowUTC().AnyTimes().Return(fixedNow)
		querier.EXPECT().AcquireLeadership(gomock.Any(), gomock.Any()).Times(1).Return(&driver.Leader{
			ElectedAt: fixedNow,
			ExpiresAt: fixedNow.Add(5 * time.Minute),
			RenewedAt: fixedNow,
			Name:      "pg_elector",
			LeaderID:  "leader-001",
			Term:      1,
		}, nil)

		querier.EXPECT().LeaderRenewal(gomock.Any(), gomock.Any()).
			AnyTimes().
			Return(nil, errors.New("database error"))
		querier.EXPECT().ResignLeadership(gomock.Any(), gomock.Any()).Times(1)

		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := elector.Start(ctx)
			assert.Error(t, err, context.Canceled)
		}()

		<-startedLeading
		assert.True(t, elector.isLeader())

		<-onStoppedLeader
		assert.True(t, elector.isFollower())

		cancel()
		wg.Wait()
	})

	t.Run("when the database layer fails when force acquiring leadership, for followers allow continuing election process till max attempts reached", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		d := mockDriver2.NewMockDriver(ctrl)
		querier := mockDriver2.NewMockQuerier(ctrl)
		mockClock := mockDriver2.NewMockClock(ctrl)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		elector, err := NewLeaderElector(ctx, d, &Config{
			ElectionClock: ElectionClock{
				LeaderDeadline:         time.Millisecond * 300,
				LeaderRetryPeriod:      time.Millisecond * 100,
				ElectionInterval:       time.Millisecond * 500,
				ElectionJitterInterval: time.Millisecond * 10,
				LeaseDuration:          time.Second * 1,
			},
			LeaderCallback: &LeaderCallback{
				OnStartedLeading: func(ctx context.Context, leader *ElectedLeader) {},
				OnStoppedLeading: func() {},
				OnNewLeader:      func(nodeId string) {},
			},
			Logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})),
			Name:            "pg_elector",
			ReleaseOnCancel: false,
		})
		assert.NoError(t, err)
		elector.clock = mockClock
		elector.maxErrAttempts = 2

		d.EXPECT().GetQuerier().AnyTimes().Return(querier)
		fixedNow := time.Now().UTC()
		mockClock.EXPECT().NowUTC().AnyTimes().Return(fixedNow)
		querier.EXPECT().AcquireLeadership(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, errors.New("database error"))

		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := elector.Start(ctx)
			assert.Error(t, err, context.Canceled)
		}()

		wg.Wait()
		cancel()
	})

	t.Run("follower path is deterministic, when force acquiring leadership was un-successful", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		d := mockDriver2.NewMockDriver(ctrl)
		querier := mockDriver2.NewMockQuerier(ctrl)
		mockClock := mockDriver2.NewMockClock(ctrl)

		stop := make(chan struct{}, 1)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		elector, err := NewLeaderElector(ctx, d, &Config{
			ElectionClock: ElectionClock{
				LeaderDeadline:         time.Millisecond * 5,
				LeaderRetryPeriod:      time.Millisecond * 2,
				ElectionInterval:       time.Millisecond * 10,
				ElectionJitterInterval: time.Millisecond * 2,
				LeaseDuration:          time.Second * 1,
			},
			LeaderCallback: &LeaderCallback{
				OnStartedLeading: func(ctx context.Context, leader *ElectedLeader) {},
				OnStoppedLeading: func() {},
				OnNewLeader:      func(nodeId string) {},
			},
			Logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})),
			Name:            "pg_elector",
			ReleaseOnCancel: false,
		})
		assert.NoError(t, err)
		elector.clock = mockClock
		elector.maxErrAttempts = 2

		d.EXPECT().GetQuerier().AnyTimes().Return(querier)
		fixedNow := time.Now().UTC()
		mockClock.EXPECT().NowUTC().AnyTimes().Return(fixedNow)

		querier.EXPECT().AcquireLeadership(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, nil).Times(2)
		querier.EXPECT().AcquireLeadership(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, driver.AcquireLeadershipParams) (*driver.Leader, error) {
			stop <- struct{}{}
			return nil, nil
		}).Times(1)

		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := elector.Start(ctx)
			assert.ErrorIs(t, err, context.Canceled)
		}()

		<-stop
		cancel()
		wg.Wait()
	})
}
