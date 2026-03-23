package driver

import (
	"context"
	"time"
)

type Driver interface {
	GetQuerier() Querier
}

type Querier interface {
	AcquireLeadership(ctx context.Context, param AcquireLeadershipParams) (*Leader, error)
	LeaderRenewal(ctx context.Context, param LeaderRenewalParams) (*Leader, error)
	ResignLeadership(ctx context.Context, param BasePrams) error
}

type BasePrams struct {
	Name     string
	LeaderId string
}

type AcquireLeadershipParams struct {
	BasePrams
	LeaseDurationSeconds float64
}

type LeaderRenewalParams struct {
	BasePrams
	LeseDuration float64
}

type Leader struct {
	Name      string
	LeaderID  string
	ElectedAt time.Time
	ExpiresAt time.Time
	RenewedAt time.Time
	Term      int64
}
