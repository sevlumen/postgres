package postgres

import (
	"context"
	"database/sql/driver"
	"sync/atomic"
)

type tx struct {
	conn *conn
	done atomic.Bool
}

func (t *tx) Commit() error {
	if !t.done.CompareAndSwap(false, true) {
		return driver.ErrBadConn
	}
	_, err := t.conn.exec(context.Background(), "COMMIT", nil)
	return err
}
func (t *tx) Rollback() error {
	if !t.done.CompareAndSwap(false, true) {
		return nil
	}
	_, err := t.conn.exec(context.Background(), "ROLLBACK", nil)
	return err
}

var _ driver.Tx = (*tx)(nil)
