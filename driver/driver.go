package driver

import "context"

type Driver interface {
	GetQuerier() Querier
}

type Querier interface {
	AcquireLeadership(ctx context.Context)
	LeaderRenewal(ctx context.Context)
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
