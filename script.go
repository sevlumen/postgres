package postgres

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"

	"github.com/sevlumen/postgres/internal/pgwire"
)

// ScriptResult describes the CommandComplete tags returned by a trusted SQL
// script in execution order.
type ScriptResult struct {
	CommandTags []string
}

// ExecScriptContext executes trusted, developer-authored SQL through the
// PostgreSQL simple-query protocol on the pinned session.
//
// This API intentionally accepts no arguments. Request or user-controlled data
// must never be interpolated into script. Use ExecContext or QueryContext for
// parameterized application values.
func (s *Session) ExecScriptContext(ctx context.Context, script string) (ScriptResult, error) {
	if err := s.validate(); err != nil {
		return ScriptResult{}, err
	}
	if ctx == nil {
		return ScriptResult{}, errors.New("postgres: script requires a context")
	}

	var result ScriptResult
	var executionErr error
	rawErr := s.conn.Raw(func(value any) error {
		connection, ok := value.(*conn)
		if !ok {
			return fmt.Errorf("postgres: unexpected driver connection %T", value)
		}
		result, executionErr = connection.execScript(ctx, script)
		return nil
	})
	if rawErr != nil {
		return ScriptResult{}, fmt.Errorf("postgres: access pinned session: %w", rawErr)
	}
	if executionErr == nil {
		return result, nil
	}
	if ctx.Err() != nil {
		_ = s.Discard()
		return result, ctx.Err()
	}
	if errors.Is(executionErr, driver.ErrBadConn) {
		_ = s.Discard()
		return result, ErrOperationOutcomeUnknown
	}
	return result, executionErr
}

func (c *conn) execScript(ctx context.Context, script string) (ScriptResult, error) {
	if c.closed.Load() || c.bad.Load() {
		return ScriptResult{}, driver.ErrBadConn
	}
	if strings.TrimSpace(script) == "" {
		return ScriptResult{}, errors.New("postgres: script is empty")
	}
	if strings.IndexByte(script, 0) >= 0 {
		return ScriptResult{}, errors.New("postgres: script contains a NUL byte")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed.Load() || c.bad.Load() {
		return ScriptResult{}, driver.ErrBadConn
	}

	clearReadDeadline := c.setReadDeadlineForContext(ctx)
	defer clearReadDeadline()
	if err := c.writeUnlocked(ctx, pgwire.Query(script)); err != nil {
		c.bad.Store(true)
		_ = c.network.Close()
		if ctx.Err() != nil {
			return ScriptResult{}, ctx.Err()
		}
		return ScriptResult{}, driver.ErrBadConn
	}
	stopCancel := c.startCancelWatcher(ctx)
	defer stopCancel()

	result := ScriptResult{CommandTags: make([]string, 0, 4)}
	var operationErr error
	for {
		message, err := c.read()
		if err != nil {
			c.bad.Store(true)
			_ = c.network.Close()
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			return result, driver.ErrBadConn
		}
		switch message.Type {
		case 'T', 'D', 'I':
			// Script execution drains row descriptions and data rows. Migration
			// scripts are not a result-reading API.
		case 'C':
			cursor := pgwire.NewCursor(message.Body)
			tag, parseErr := cursor.CString()
			if parseErr != nil {
				c.bad.Store(true)
				_ = c.network.Close()
				return result, parseErr
			}
			result.CommandTags = append(result.CommandTags, tag)
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
				_ = c.network.Close()
				return result, parseErr
			}
			c.parameters[name] = value
		case 'K':
			cursor := pgwire.NewCursor(message.Body)
			pid, parseErr := cursor.Int32()
			if parseErr != nil {
				c.bad.Store(true)
				_ = c.network.Close()
				return result, parseErr
			}
			secret := append([]byte(nil), cursor.Rest()...)
			if len(secret) != 4 {
				c.bad.Store(true)
				_ = c.network.Close()
				return result, fmt.Errorf("postgres: protocol 3.0 expected 4-byte cancel key, got %d", len(secret))
			}
			c.backendPID, c.cancelKey = pid, secret
		case 'A':
			// Notifications are ignored until LISTEN/NOTIFY becomes public API.
		case 'G', 'H', 'W':
			c.bad.Store(true)
			_ = c.network.Close()
			return result, errors.New("postgres: COPY is not supported by script execution")
		case 'Z':
			if len(message.Body) != 1 {
				c.bad.Store(true)
				_ = c.network.Close()
				return result, errors.New("postgres: malformed ReadyForQuery")
			}
			c.txStatus = message.Body[0]
			if ctx.Err() != nil {
				c.bad.Store(true)
				_ = c.network.Close()
				return result, ctx.Err()
			}
			if operationErr != nil {
				return result, operationErr
			}
			return result, nil
		default:
			c.bad.Store(true)
			_ = c.network.Close()
			return result, fmt.Errorf("postgres: unexpected script response %q", message.Type)
		}
	}
}
