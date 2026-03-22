package databasesql

import (
	"context"
	"database/sql"

	"github.com/TimKotowski/pg_elector/driver"
)

var _ driver.Driver = (*Driver)(nil)

type Driver struct {
	pool *sql.DB
}

type Querier struct {
	driver *Driver
}

func New(pool *sql.DB) *Driver {
	return &Driver{pool: pool}
}

func (d *Driver) GetQuerier() driver.Querier {
	return &Querier{driver: d}
}

func (q Querier) AcquireLeadership(ctx context.Context, param driver.AcquireLeadershipParams) (bool, error) {
	return false, nil
}

func (q Querier) LeaderRenewal(ctx context.Context, param driver.LeaderRenewalParams) (int64, error) {
	return 0, nil
}

func (q Querier) ReleaseLeadership(ctx context.Context, param driver.BasePrams) error {
	return nil
}
