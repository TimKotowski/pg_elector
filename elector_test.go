package pg_elector

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/TimKotowski/pg_elector/driver"
	"github.com/TimKotowski/pg_elector/driver/mockDriver"
)

func TestSingleNodeElector(t *testing.T) {
	t.Run("when ReleaseOnCancel is true, leader node revoked leadership immediately on context cancel", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		d := mockDriver.NewMockDriver(ctrl)
		querier := mockDriver.NewMockQuerier(ctrl)

		startedLeading := make(chan struct{}, 1)
		onStoppedLeader := make(chan struct{}, 1)
		ctx, cancel := context.WithCancel(context.Background())
		elector, err := NewLeaderElector(ctx, d, &Config{
			ElectionClock: ElectionClock{
				LeaderDeadline:         time.Second * 2,
				LeaderRetryPeriod:      time.Millisecond * 100,
				ElectionInterval:       time.Second * 4,
				ElectionJitterInterval: time.Millisecond * 80,
			},
			LeaderCallback: LeaderCallback{
				OnStartedLeading: func(ctx context.Context) {
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
			RenewedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			Name:      "pg_elector",
			LeaderID:  "leader-001",
		}, nil)
		querier.EXPECT().LeaderRenewal(gomock.Any(), gomock.Any()).AnyTimes().Return(&driver.Leader{
			ElectedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
			RenewedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			Name:      "pg_elector",
			LeaderID:  "leader-001",
		}, nil)
		querier.EXPECT().ReleaseLeadership(gomock.Any(), gomock.Any()).Times(1).Return(nil)

		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			err := elector.Start(ctx)
			assert.ErrorIs(t, err, context.Canceled)
		}(wg)

		<-startedLeading
		assert.True(t, elector.isLeader())

		wait := time.NewTimer(elector.config.ElectionClock.LeaderRetryPeriod)
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
		d := mockDriver.NewMockDriver(ctrl)
		querier := mockDriver.NewMockQuerier(ctrl)

		startedLeading := make(chan struct{}, 1)
		onStoppedLeader := make(chan struct{}, 1)
		ctx, cancel := context.WithCancel(context.Background())
		elector, err := NewLeaderElector(ctx, d, &Config{
			ElectionClock: ElectionClock{
				LeaderDeadline:         time.Second * 2,
				LeaderRetryPeriod:      time.Millisecond * 100,
				ElectionInterval:       time.Second * 4,
				ElectionJitterInterval: time.Millisecond * 80,
			},
			LeaderCallback: LeaderCallback{
				OnStartedLeading: func(ctx context.Context) {
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
			RenewedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			Name:      "pg_elector",
			LeaderID:  "leader-001",
		}, nil)
		querier.EXPECT().LeaderRenewal(gomock.Any(), gomock.Any()).AnyTimes().Return(&driver.Leader{
			ElectedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
			RenewedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			Name:      "pg_elector",
			LeaderID:  "leader-001",
		}, nil)
		querier.EXPECT().ReleaseLeadership(gomock.Any(), gomock.Any()).Times(0).Return(nil)

		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			err := elector.Start(ctx)
			assert.ErrorIs(t, err, context.Canceled)
		}(wg)

		<-startedLeading
		assert.True(t, elector.isLeader())

		wait := time.NewTimer(elector.config.ElectionClock.LeaderRetryPeriod)
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
		d := mockDriver.NewMockDriver(ctrl)
		querier := mockDriver.NewMockQuerier(ctrl)

		startedLeading := make(chan struct{}, 1)
		onStoppedLeader := make(chan struct{}, 1)
		ctx, cancel := context.WithCancel(context.Background())
		elector, err := NewLeaderElector(ctx, d, &Config{
			ElectionClock: ElectionClock{
				LeaderDeadline:         time.Second * 2,
				LeaderRetryPeriod:      time.Millisecond * 100,
				ElectionInterval:       time.Second * 10,
				ElectionJitterInterval: time.Millisecond * 80,
			},
			LeaderCallback: LeaderCallback{
				OnStartedLeading: func(ctx context.Context) {
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
		mockClock := mockDriver.NewMockClock(ctrl)
		elector.clock = mockClock
		assert.NoError(t, err)

		d.EXPECT().GetQuerier().AnyTimes().Return(querier)

		mockClock.EXPECT().NowUTC().AnyTimes().Return(elector.clock.NowUTC().Add(time.Minute * 1))
		querier.EXPECT().AcquireLeadership(gomock.Any(), gomock.Any()).Return(&driver.Leader{
			ElectedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
			RenewedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			Name:      "pg_elector",
			LeaderID:  "leader-001",
		}, nil)
		querier.EXPECT().LeaderRenewal(gomock.Any(), gomock.Any()).AnyTimes().Return(&driver.Leader{
			ElectedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
			RenewedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			Name:      "pg_elector",
			LeaderID:  "leader-001",
		}, nil)

		querier.EXPECT().ReleaseLeadership(gomock.Any(), gomock.Any()).Times(0)

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
		timer := time.NewTimer(elector.config.ElectionClock.LeaderDeadline * 2)
		<-timer.C
		assert.True(t, elector.isLeader())

		cancel()
		<-onStoppedLeader
	})

	t.Run("leader resigns when renewal was revoked", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		d := mockDriver.NewMockDriver(ctrl)
		querier := mockDriver.NewMockQuerier(ctrl)

		startedLeading := make(chan struct{}, 1)
		onStoppedLeader := make(chan struct{}, 1)
		ctx, cancel := context.WithCancel(context.Background())
		elector, err := NewLeaderElector(ctx, d, &Config{
			ElectionClock: ElectionClock{
				LeaderDeadline:         time.Second * 2,
				LeaderRetryPeriod:      time.Millisecond * 1,
				ElectionInterval:       time.Second * 3,
				ElectionJitterInterval: time.Millisecond * 1,
			},
			LeaderCallback: LeaderCallback{
				OnStartedLeading: func(ctx context.Context) {
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
		mockClock := mockDriver.NewMockClock(ctrl)
		elector.clock = mockClock
		assert.NoError(t, err)

		d.EXPECT().GetQuerier().AnyTimes().Return(querier)
		mockClock.EXPECT().NowUTC().AnyTimes().Return(elector.clock.NowUTC().Add(time.Minute * 1))
		querier.EXPECT().AcquireLeadership(gomock.Any(), gomock.Any()).Times(1).Return(&driver.Leader{
			ElectedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
			RenewedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
			Name:      "pg_elector",
			LeaderID:  "leader-001",
		}, nil)

		gomock.InOrder(
			querier.EXPECT().LeaderRenewal(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, params driver.LeaderRenewalParams) (*driver.Leader, error) {
					return &driver.Leader{
						ElectedAt: time.Now().UTC(),
						ExpiresAt: time.Now().UTC().Add(5 * time.Minute),
						RenewedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
						Name:      "pg_elector",
						LeaderID:  "leader-001",
					}, nil
				}),
			querier.EXPECT().LeaderRenewal(gomock.Any(), gomock.Any()).Return(nil, nil),
		)
		querier.EXPECT().ReleaseLeadership(gomock.Any(), gomock.Any()).Times(0)
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
	//t.Run("leader steps down when LeaderDeadline is reached", func(t *testing.T) {})
	//t.Run("leader steps down when LeaderDeadline has been reached, during long running renewal query", func(t *testing.T) {})
	//
	//t.Run("when the database layer fails, for leader allow continuing election process till max attempts reached", func(t *testing.T) {})
	//t.Run("when the database layer fails, for followers allow continuing election process till max attempts reached", func(t *testing.T) {})
	//
	//t.Run("follower retries acquiring leader after failed attempt without crashing", func(t *testing.T) {})
	t.Run("follower remains follower when acquire returns false with no error", func(t *testing.T) {})
}

//func MultiNodeElector(t *testing.T) {
//	t.Run("multiple nodes spawn, with at most once, leadership acquired", func(t *testing.T) {})
//	t.Run("when leader losses leadership, a new node takes leadership", func(t *testing.T) {})
//}
//
//func TestElectorDuration(t *testing.T) {
//	t.Run("leaseDuration returns interval plus 50 percent padding for short intervals", func(t *testing.T) {})
//	t.Run("leaseDuration clamps padding to minimum 10 seconds", func(t *testing.T) {})
//	t.Run("leaseDuration reduces padding ratio when padding exceeds 2 minutes", func(t *testing.T) {})
//	t.Run("JitterDuration output is always within 0.5x to 1.1x of input", func(t *testing.T) {})
//}
