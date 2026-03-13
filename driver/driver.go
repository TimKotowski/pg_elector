package driver

import "context"

type Driver interface {
	GetQuerier() Querier
}

type Querier interface {
	AcquireLeadership(ctx context.Context, param AcquireLeadershipParams) (bool, error)
	LeaderRenewal(ctx context.Context, param LeaderRenewalParams) (int64, error)
}

type BasePrams struct {
	Name     string
	LeaderId string
}

type AcquireLeadershipParams struct {
	BasePrams
	LeseDuration float64
}

type LeaderRenewalParams struct {
	BasePrams
	LeaderId string
}
