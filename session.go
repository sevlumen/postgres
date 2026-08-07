package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"sync/atomic"
)

// Row is the minimal scan surface shared by sql.Row and ORM-generated scanners.
type Row interface {
	Scan(dest ...any) error
}

// Session owns one pinned database/sql connection until Close or Discard.
//
// Sessions are intended for PostgreSQL session-scoped operations such as
// advisory locks and migration execution. A Session must not be used
// concurrently by multiple goroutines.
type Session struct {
	conn   *sql.Conn
	closed atomic.Bool
}

// Acquire obtains one pinned connection from db.
func Acquire(ctx context.Context, db *sql.DB) (*Session, error) {
	if db == nil {
		return nil, errors.New("postgres: session requires a database")
	}
	if ctx == nil {
		return nil, errors.New("postgres: session requires a context")
	}
	connection, err := db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("postgres: acquire session: %w", err)
	}
	return &Session{conn: connection}, nil
}

// ExecContext executes one parameterized statement on the pinned connection.
func (s *Session) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	return s.conn.ExecContext(ctx, query, args...)
}

// QueryContext executes one parameterized query on the pinned connection.
func (s *Session) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	return s.conn.QueryContext(ctx, query, args...)
}

// QueryRowContext executes one parameterized query and returns its first row.
func (s *Session) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	if err := s.validate(); err != nil {
		return errorRow{err: err}
	}
	return s.conn.QueryRowContext(ctx, query, args...)
}

// BeginTx begins a database/sql transaction on the pinned connection.
func (s *Session) BeginTx(ctx context.Context, options *sql.TxOptions) (*sql.Tx, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	return s.conn.BeginTx(ctx, options)
}

// Close returns the pinned connection to the database/sql pool.
func (s *Session) Close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	return s.conn.Close()
}

// Discard permanently removes the pinned backend from the database/sql pool.
// It is intended for failed advisory-lock cleanup, protocol uncertainty, or
// any other situation where returning the backend to the pool is unsafe.
func (s *Session) Discard() error {
	if s == nil || s.conn == nil {
		return nil
	}
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}

	rawErr := s.conn.Raw(func(value any) error {
		connection, ok := value.(*conn)
		if !ok {
			return fmt.Errorf("postgres: unexpected driver connection %T", value)
		}
		connection.bad.Store(true)
		_ = connection.network.Close()
		return driver.ErrBadConn
	})
	if errors.Is(rawErr, driver.ErrBadConn) {
		rawErr = nil
	}
	return errors.Join(rawErr, s.conn.Close())
}

func (s *Session) validate() error {
	if s == nil || s.conn == nil {
		return errors.New("postgres: session is not configured")
	}
	if s.closed.Load() {
		return errors.New("postgres: session is closed")
	}
	return nil
}

type errorRow struct{ err error }

func (r errorRow) Scan(...any) error { return r.err }
