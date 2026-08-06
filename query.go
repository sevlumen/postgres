package postgres

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"

	"github.com/sevlumen/postgres/internal/pgwire"
)

func (c *conn) exec(ctx context.Context, query string, encoded *encodedArguments) (driver.Result, error) {
	if c.closed.Load() || c.bad.Load() {
		return nil, driver.ErrBadConn
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed.Load() || c.bad.Load() {
		return nil, driver.ErrBadConn
	}

	var values [][]byte
	var nulls []bool
	if encoded != nil {
		values, nulls = encoded.values, encoded.nulls
	}
	clearReadDeadline := c.setReadDeadlineForContext(ctx)
	defer clearReadDeadline()
	payload, err := pgwire.ExtendedQuery(query, values, nulls)
	if err != nil {
		return nil, err
	}
	if err := c.writeUnlocked(ctx, payload); err != nil {
		c.bad.Store(true)
		return nil, driver.ErrBadConn
	}
	stopCancel := c.startCancelWatcher(ctx)
	defer stopCancel()

	var tag string
	var operationErr error
	for {
		message, err := c.read()
		if err != nil {
			c.bad.Store(true)
			return nil, driver.ErrBadConn
		}
		switch message.Type {
		case '1', '2', 'n', 'I':
		case 'T':
			// Exec drains rows for statements such as INSERT ... RETURNING.
		case 'D':
		case 'C':
			cursor := pgwire.NewCursor(message.Body)
			tag, err = cursor.CString()
			if err != nil {
				c.bad.Store(true)
				return nil, err
			}
		case 'E':
			if operationErr == nil {
				operationErr = parseError(message.Body)
			}
		case 'N':
			c.lastNotice = parseError(message.Body)
		case 'S':
			name, value, parseErr := parseParameterStatus(message.Body)
			if parseErr != nil {
				c.bad.Store(true)
				return nil, parseErr
			}
			c.parameters[name] = value
		case 'Z':
			if len(message.Body) != 1 {
				c.bad.Store(true)
				return nil, errors.New("postgres: malformed ReadyForQuery")
			}
			c.txStatus = message.Body[0]
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if operationErr != nil {
				return nil, operationErr
			}
			return commandResult{tag: tag}, nil
		case 'A':
			// Notifications are ignored until LISTEN/NOTIFY becomes public API.
		default:
			c.bad.Store(true)
			return nil, fmt.Errorf("postgres: unexpected exec response %q", message.Type)
		}
	}
}

func (c *conn) query(ctx context.Context, query string, encoded *encodedArguments) (driver.Rows, error) {
	if c.closed.Load() || c.bad.Load() {
		return nil, driver.ErrBadConn
	}
	c.mu.Lock()
	if c.closed.Load() || c.bad.Load() {
		c.mu.Unlock()
		return nil, driver.ErrBadConn
	}

	var values [][]byte
	var nulls []bool
	if encoded != nil {
		values, nulls = encoded.values, encoded.nulls
	}
	clearReadDeadline := c.setReadDeadlineForContext(ctx)
	payload, err := pgwire.ExtendedQuery(query, values, nulls)
	if err != nil {
		clearReadDeadline()
		c.mu.Unlock()
		return nil, err
	}
	if err := c.writeUnlocked(ctx, payload); err != nil {
		clearReadDeadline()
		c.bad.Store(true)
		c.mu.Unlock()
		return nil, driver.ErrBadConn
	}
	result := &rows{conn: c, ctx: ctx, stopCancel: c.startCancelWatcher(ctx), clearDeadline: clearReadDeadline, locked: true}
	for {
		message, err := c.read()
		if err != nil {
			result.finishBad()
			return nil, driver.ErrBadConn
		}
		switch message.Type {
		case '1', '2':
		case 'T':
			fields, parseErr := parseRowDescription(message.Body)
			if parseErr != nil {
				result.finishBad()
				return nil, parseErr
			}
			result.fields = fields
			return result, nil
		case 'n':
			// A statement without rows still returns an empty Rows.
		case 'C':
			cursor := pgwire.NewCursor(message.Body)
			result.commandTag, _ = cursor.CString()
		case 'E':
			result.operationErr = parseError(message.Body)
		case 'N':
			c.lastNotice = parseError(message.Body)
		case 'Z':
			if len(message.Body) != 1 {
				result.finishBad()
				return nil, errors.New("postgres: malformed ReadyForQuery")
			}
			c.txStatus = message.Body[0]
			result.finish()
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if result.operationErr != nil {
				return nil, result.operationErr
			}
			return &rows{done: true}, nil
		default:
			result.finishBad()
			return nil, fmt.Errorf("postgres: unexpected query response %q", message.Type)
		}
	}
}

func parseError(body []byte) *Error {
	result := &Error{}
	cursor := pgwire.NewCursor(body)
	for cursor.Remaining() > 0 {
		field, err := cursor.Byte()
		if err != nil {
			break
		}
		if field == 0 {
			break
		}
		value, err := cursor.CString()
		if err != nil {
			break
		}
		switch field {
		case 'S':
			result.Severity = value
		case 'V':
			result.SeverityNonLocal = value
		case 'C':
			result.Code = value
		case 'M':
			result.Message = value
		case 'D':
			result.Detail = value
		case 'H':
			result.Hint = value
		case 'P':
			result.Position = value
		case 'p':
			result.InternalPosition = value
		case 'q':
			result.InternalQuery = value
		case 'W':
			result.Where = value
		case 's':
			result.Schema = value
		case 't':
			result.Table = value
		case 'c':
			result.Column = value
		case 'd':
			result.DataType = value
		case 'n':
			result.Constraint = value
		case 'F':
			result.File = value
		case 'L':
			result.Line = value
		case 'R':
			result.Routine = value
		}
	}
	if result.Message == "" {
		result.Message = "unknown PostgreSQL error"
	}
	return result
}

var _ = io.EOF
