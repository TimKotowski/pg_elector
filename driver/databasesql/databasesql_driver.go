package databasesql

import (
	"database/sql"

	"github.com/TimKotowski/pg_elector/driver"
)

var _ driver.Driver = (*Driver)(nil)

type Driver struct {
	pool *sql.DB
}

func New(pool *sql.DB) *Driver {
	return &Driver{pool: pool}
}

func (d *Driver) GetQuerier() driver.Querier {
	return nil
}
