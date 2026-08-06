package postgres

import (
	"context"
	"database/sql/driver"
)

type stmt struct {
	conn  *conn
	query string
}

func (s *stmt) Close() error  { return nil }
func (s *stmt) NumInput() int { return -1 }
func (s *stmt) Exec(args []driver.Value) (driver.Result, error) {
	named := make([]driver.NamedValue, len(args))
	for i, value := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: value}
	}
	return s.ExecContext(context.Background(), named)
}
func (s *stmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, value := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: value}
	}
	return s.QueryContext(context.Background(), named)
}
func (s *stmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	return s.conn.ExecContext(ctx, s.query, args)
}
func (s *stmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	return s.conn.QueryContext(ctx, s.query, args)
}

var _ driver.Stmt = (*stmt)(nil)
var _ driver.StmtExecContext = (*stmt)(nil)
var _ driver.StmtQueryContext = (*stmt)(nil)
