package pgxv5

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TimKotowski/pg_elector/driver"
)

var _ driver.Driver = (*Driver)(nil)

type Driver struct {
	pool *pgxpool.Pool
}

type Querier struct {
	driver *Driver
}

func New(pool *pgxpool.Pool) *Driver {
	return &Driver{pool: pool}
}

func (d *Driver) GetQuerier() driver.Querier {
	return &Querier{driver: d}
}

func (q *Querier) AcquireLeadership(ctx context.Context) {
}

func (q *Querier) LeaderRenewal(ctx context.Context) {
}
