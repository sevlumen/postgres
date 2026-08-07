// Package stdlib registers the sevlumen-postgres database/sql driver.
package stdlib

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"sync"

	"github.com/sevlumen/postgres"
)

const DriverName = "sevlumen-postgres"

var once sync.Once

func init() { Register() }

func Register() {
	once.Do(func() { sql.Register(DriverName, driverAdapter{}) })
}

type driverAdapter struct{}

func (driverAdapter) Open(name string) (driver.Conn, error) {
	config, err := postgres.ParseConfig(name)
	if err != nil {
		return nil, err
	}
	connector, err := postgres.NewConnector(config)
	if err != nil {
		return nil, err
	}
	return connector.Connect(context.Background())
}

var _ driver.Driver = driverAdapter{}
