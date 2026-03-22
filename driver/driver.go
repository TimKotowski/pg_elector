package driver

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type Driver interface {
	GetQuerier() Querier
}

type Querier interface {
	AcquireLeadership(ctx context.Context, param AcquireLeadershipParams) (*Leader, error)
	LeaderRenewal(ctx context.Context, param LeaderRenewalParams) (*Leader, error)
	ReleaseLeadership(ctx context.Context, param BasePrams) error
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
	ElectedAt time.Time
	ExpiresAt time.Time
	RenewedAt pgtype.Timestamptz
	Name      string
	LeaderID  string
}
