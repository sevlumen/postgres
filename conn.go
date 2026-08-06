package postgres

import (
	"bufio"
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sevlumen/postgres/internal/pgwire"
)

type conn struct {
	config  Config
	network net.Conn
	reader  *bufio.Reader
	writer  *bufio.Writer
	secure  bool

	mu         sync.Mutex
	closed     atomic.Bool
	bad        atomic.Bool
	txStatus   byte
	backendPID int32
	cancelKey  []byte
	parameters map[string]string
	lastNotice *Error
}

func newConn(network net.Conn, config Config, secure bool) *conn {
	return &conn{
		config:     config,
		network:    network,
		reader:     bufio.NewReaderSize(network, 32<<10),
		writer:     bufio.NewWriterSize(network, 32<<10),
		secure:     secure,
		txStatus:   'I',
		parameters: make(map[string]string),
	}
}

func (c *conn) Prepare(query string) (driver.Stmt, error) {
	return c.PrepareContext(context.Background(), query)
}
func (c *conn) PrepareContext(_ context.Context, query string) (driver.Stmt, error) {
	if c.closed.Load() {
		return nil, driver.ErrBadConn
	}
	return &stmt{conn: c, query: query}, nil
}

func (c *conn) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.writeUnlocked(context.Background(), pgwire.Terminate())
	return c.network.Close()
}

func (c *conn) Begin() (driver.Tx, error) { return c.BeginTx(context.Background(), driver.TxOptions{}) }
func (c *conn) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	if c.closed.Load() || c.bad.Load() {
		return nil, driver.ErrBadConn
	}
	isolation, err := isolationClause(options.Isolation)
	if err != nil {
		return nil, err
	}
	query := "BEGIN"
	if isolation != "" {
		query += " ISOLATION LEVEL " + isolation
	}
	if options.ReadOnly {
		query += " READ ONLY"
	}
	if _, err := c.exec(ctx, query, nil); err != nil {
		return nil, err
	}
	return &tx{conn: c}, nil
}

func isolationClause(level driver.IsolationLevel) (string, error) {
	switch level {
	case driver.IsolationLevel(0):
		return "", nil
	case driver.IsolationLevel(1):
		return "READ UNCOMMITTED", nil
	case driver.IsolationLevel(2):
		return "READ COMMITTED", nil
	case driver.IsolationLevel(4):
		return "REPEATABLE READ", nil
	case driver.IsolationLevel(6):
		return "SERIALIZABLE", nil
	default:
		return "", fmt.Errorf("postgres: unsupported isolation level %d", level)
	}
}

func (c *conn) Ping(ctx context.Context) error {
	_, err := c.exec(ctx, "SELECT 1", nil)
	return err
}

func (c *conn) ResetSession(ctx context.Context) error {
	if c.closed.Load() || c.bad.Load() {
		return driver.ErrBadConn
	}
	if c.txStatus == 'T' || c.txStatus == 'E' {
		if _, err := c.exec(ctx, "ROLLBACK", nil); err != nil {
			c.bad.Store(true)
			return driver.ErrBadConn
		}
	}
	return nil
}

func (c *conn) IsValid() bool { return !c.closed.Load() && !c.bad.Load() }

func (c *conn) CheckNamedValue(value *driver.NamedValue) error {
	converted, err := normalizeValue(value.Value)
	if err != nil {
		return err
	}
	value.Value = converted
	return nil
}

func (c *conn) Exec(query string, args []driver.Value) (driver.Result, error) {
	named := make([]driver.NamedValue, len(args))
	for i, value := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: value}
	}
	return c.ExecContext(context.Background(), query, named)
}
func (c *conn) Query(query string, args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, value := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: value}
	}
	return c.QueryContext(context.Background(), query, named)
}
func (c *conn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	encoded, err := encodeArguments(args)
	if err != nil {
		return nil, err
	}
	return c.exec(ctx, query, encoded)
}
func (c *conn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	encoded, err := encodeArguments(args)
	if err != nil {
		return nil, err
	}
	return c.query(ctx, query, encoded)
}

func (c *conn) write(ctx context.Context, payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeUnlocked(ctx, payload)
}
func (c *conn) writeUnlocked(ctx context.Context, payload []byte) error {
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.network.SetWriteDeadline(deadline)
		defer c.network.SetWriteDeadline(time.Time{})
	}
	if _, err := c.writer.Write(payload); err != nil {
		return err
	}
	return c.writer.Flush()
}
func (c *conn) read() (pgwire.Message, error) {
	message, err := pgwire.ReadMessage(c.reader, c.config.MaxMessageSize)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
			c.bad.Store(true)
		}
	}
	return message, err
}

func (c *conn) setReadDeadlineForContext(ctx context.Context) func() {
	if deadline, ok := ctx.Deadline(); ok {
		_ = c.network.SetReadDeadline(deadline.Add(c.config.CancelTimeout))
		return func() { _ = c.network.SetReadDeadline(time.Time{}) }
	}
	return func() {}
}

func (c *conn) startCancelWatcher(ctx context.Context) func() {
	if ctx.Done() == nil {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			cancelCtx, cancel := context.WithTimeout(context.Background(), c.config.CancelTimeout)
			_ = c.cancel(cancelCtx)
			cancel()
		case <-done:
		}
	}()
	return func() { close(done) }
}

func (c *conn) cancel(ctx context.Context) error {
	if c.backendPID == 0 || len(c.cancelKey) == 0 {
		return nil
	}
	dialer := net.Dialer{Timeout: c.config.CancelTimeout}
	connection, err := dialer.DialContext(ctx, "tcp", c.config.address())
	if err != nil {
		return err
	}
	defer connection.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}
	_, err = connection.Write(pgwire.CancelRequest(c.backendPID, c.cancelKey))
	return err
}

func (c *conn) drainUntilReady() error {
	var firstErr error
	for {
		message, err := c.read()
		if err != nil {
			return err
		}
		switch message.Type {
		case 'E':
			if firstErr == nil {
				firstErr = parseError(message.Body)
			}
		case 'N':
			c.lastNotice = parseError(message.Body)
		case 'S':
			name, value, parseErr := parseParameterStatus(message.Body)
			if parseErr != nil {
				return parseErr
			}
			c.parameters[name] = value
		case 'K':
			cursor := pgwire.NewCursor(message.Body)
			pid, parseErr := cursor.Int32()
			if parseErr != nil {
				return parseErr
			}
			c.backendPID, c.cancelKey = pid, append([]byte(nil), cursor.Rest()...)
		case 'Z':
			if len(message.Body) != 1 {
				return errors.New("postgres: malformed ReadyForQuery")
			}
			c.txStatus = message.Body[0]
			return firstErr
		}
	}
}

var (
	_ driver.Conn               = (*conn)(nil)
	_ driver.ConnPrepareContext = (*conn)(nil)
	_ driver.ConnBeginTx        = (*conn)(nil)
	_ driver.ExecerContext      = (*conn)(nil)
	_ driver.QueryerContext     = (*conn)(nil)
	_ driver.Pinger             = (*conn)(nil)
	_ driver.SessionResetter    = (*conn)(nil)
	_ driver.Validator          = (*conn)(nil)
	_ driver.NamedValueChecker  = (*conn)(nil)
)
