package postgres

import (
	"context"
	"database/sql/driver"
	"sync/atomic"
)

type tx struct {
	conn *conn
	ctx  context.Context
	done atomic.Bool
}

func (t *tx) Commit() error {
	if !t.done.CompareAndSwap(false, true) {
		return driver.ErrBadConn
	}
	ctx := t.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := t.conn.exec(ctx, "COMMIT", nil)
	return err
}

func (t *tx) Rollback() error {
	if !t.done.CompareAndSwap(false, true) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), t.conn.config.CancelTimeout)
	defer cancel()
	_, err := t.conn.exec(ctx, "ROLLBACK", nil)
	return err
}

var _ driver.Tx = (*tx)(nil)
