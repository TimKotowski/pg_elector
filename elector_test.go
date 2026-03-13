package pg_elector

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/TimKotowski/pg_elector/driver/mockDriver"
)

func TestElector(t *testing.T) {
	t.Run("elector obtains leadership, then releases on cancel from ctx deadline", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		driver := mockDriver.NewMockDriver(ctrl)
		querier := mockDriver.NewMockQuerier(ctrl)

		elector, err := NewLeaderElector(t.Context(), driver, &Config{
			ElectionClock: ElectionClock{
				LeaderDeadline:         time.Second * 1,
				LeadRetryPeriod:        time.Millisecond * 500,
				ElectionInterval:       time.Second * 2,
				ElectionJitterInterval: time.Millisecond * 10,
			},
			Name:            "default",
			ReleaseOnCancel: true,
		})
		if err != nil {
			return
		}

		driver.EXPECT().GetQuerier().AnyTimes().Return(querier)
		querier.EXPECT().AcquireLeadership(gomock.Any(), gomock.Any()).Return(true, nil)
		querier.EXPECT().LeaderRenewal(gomock.Any(), gomock.Any()).AnyTimes().Return(int64(1), nil)

		ctx, cancel := context.WithTimeout(t.Context(), time.Second*4)
		err = elector.Start(ctx)
		cancel()

		assert.Eventually(t, func() bool {
			return assert.ErrorIs(t, err, context.DeadlineExceeded)
		}, time.Second, time.Millisecond*10)
	})
}
