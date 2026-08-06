package postgres

import (
	"context"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/sevlumen/postgres/internal/pgwire"
)

type fieldDescription struct {
	Name         string
	TableOID     uint32
	Attribute    int16
	TypeOID      uint32
	TypeSize     int16
	TypeModifier int32
	Format       int16
}

type rows struct {
	conn          *conn
	ctx           context.Context
	fields        []fieldDescription
	commandTag    string
	operationErr  error
	stopCancel    func()
	clearDeadline func()
	locked        bool
	done          bool
}

func (r *rows) Columns() []string {
	columns := make([]string, len(r.fields))
	for i := range r.fields {
		columns[i] = r.fields[i].Name
	}
	return columns
}

func (r *rows) Close() error {
	if r == nil || r.done {
		return nil
	}
	if r.conn == nil {
		r.done = true
		return nil
	}
	cancelCtx, cancel := context.WithTimeout(context.Background(), r.conn.config.CancelTimeout)
	_ = r.conn.cancel(cancelCtx)
	cancel()
	deadline := time.Now().Add(r.conn.config.CancelTimeout)
	_ = r.conn.network.SetReadDeadline(deadline)
	defer r.conn.network.SetReadDeadline(time.Time{})
	for {
		message, err := r.conn.read()
		if err != nil {
			r.finishBad()
			return nil
		}
		switch message.Type {
		case 'E':
			if r.operationErr == nil {
				r.operationErr = parseError(message.Body)
			}
		case 'N':
			r.conn.lastNotice = parseError(message.Body)
		case 'Z':
			if len(message.Body) == 1 {
				r.conn.txStatus = message.Body[0]
			}
			r.finish()
			return nil
		}
	}
}

func (r *rows) Next(dest []driver.Value) error {
	if r == nil || r.done {
		return io.EOF
	}
	for {
		message, err := r.conn.read()
		if err != nil {
			r.finishBad()
			return driver.ErrBadConn
		}
		switch message.Type {
		case 'D':
			values, parseErr := parseDataRow(message.Body, r.fields)
			if parseErr != nil {
				r.finishBad()
				return parseErr
			}
			if len(dest) != len(values) {
				r.finishBad()
				return fmt.Errorf("postgres: destination has %d columns, row has %d", len(dest), len(values))
			}
			copy(dest, values)
			return nil
		case 'C':
			cursor := pgwire.NewCursor(message.Body)
			r.commandTag, _ = cursor.CString()
		case 'E':
			if r.operationErr == nil {
				r.operationErr = parseError(message.Body)
			}
		case 'N':
			r.conn.lastNotice = parseError(message.Body)
		case 'S':
			name, value, parseErr := parseParameterStatus(message.Body)
			if parseErr != nil {
				r.finishBad()
				return parseErr
			}
			r.conn.parameters[name] = value
		case 'Z':
			if len(message.Body) != 1 {
				r.finishBad()
				return errors.New("postgres: malformed ReadyForQuery")
			}
			r.conn.txStatus = message.Body[0]
			r.finish()
			if r.ctx != nil && r.ctx.Err() != nil {
				return r.ctx.Err()
			}
			if r.operationErr != nil {
				return r.operationErr
			}
			return io.EOF
		case 'A':
		default:
			r.finishBad()
			return fmt.Errorf("postgres: unexpected rows response %q", message.Type)
		}
	}
}

func (r *rows) ColumnTypeDatabaseTypeName(index int) string {
	if index < 0 || index >= len(r.fields) {
		return ""
	}
	return databaseTypeName(r.fields[index].TypeOID)
}
func (r *rows) ColumnTypeLength(index int) (int64, bool) {
	if index < 0 || index >= len(r.fields) {
		return 0, false
	}
	size := r.fields[index].TypeSize
	if size < 0 {
		return 0, false
	}
	return int64(size), true
}
func (r *rows) ColumnTypeNullable(_ int) (bool, bool) { return true, false }
func (r *rows) HasNextResultSet() bool                { return false }
func (r *rows) NextResultSet() error                  { return io.EOF }

func (r *rows) finish() {
	if r.done {
		return
	}
	r.done = true
	if r.stopCancel != nil {
		r.stopCancel()
		r.stopCancel = nil
	}
	if r.clearDeadline != nil {
		r.clearDeadline()
		r.clearDeadline = nil
	}
	if r.locked && r.conn != nil {
		r.locked = false
		r.conn.mu.Unlock()
	}
}
func (r *rows) finishBad() {
	if r.conn != nil {
		r.conn.bad.Store(true)
		_ = r.conn.network.Close()
	}
	r.finish()
}

func parseRowDescription(body []byte) ([]fieldDescription, error) {
	cursor := pgwire.NewCursor(body)
	count, err := cursor.Int16()
	if err != nil {
		return nil, err
	}
	if count < 0 || count > 32767 {
		return nil, errors.New("postgres: invalid row field count")
	}
	fields := make([]fieldDescription, int(count))
	for i := range fields {
		name, err := cursor.CString()
		if err != nil {
			return nil, err
		}
		tableOID, err := cursor.Uint32()
		if err != nil {
			return nil, err
		}
		attribute, err := cursor.Int16()
		if err != nil {
			return nil, err
		}
		typeOID, err := cursor.Uint32()
		if err != nil {
			return nil, err
		}
		typeSize, err := cursor.Int16()
		if err != nil {
			return nil, err
		}
		typeModifier, err := cursor.Int32()
		if err != nil {
			return nil, err
		}
		format, err := cursor.Int16()
		if err != nil {
			return nil, err
		}
		if format != 0 && format != 1 {
			return nil, fmt.Errorf("postgres: unsupported field format %d", format)
		}
		fields[i] = fieldDescription{Name: name, TableOID: tableOID, Attribute: attribute, TypeOID: typeOID, TypeSize: typeSize, TypeModifier: typeModifier, Format: format}
	}
	if cursor.Remaining() != 0 {
		return nil, errors.New("postgres: trailing RowDescription data")
	}
	return fields, nil
}

func parseDataRow(body []byte, fields []fieldDescription) ([]driver.Value, error) {
	cursor := pgwire.NewCursor(body)
	count, err := cursor.Int16()
	if err != nil {
		return nil, err
	}
	if int(count) != len(fields) {
		return nil, fmt.Errorf("postgres: DataRow has %d values, expected %d", count, len(fields))
	}
	values := make([]driver.Value, int(count))
	for i := range values {
		length, err := cursor.Int32()
		if err != nil {
			return nil, err
		}
		if length == -1 {
			values[i] = nil
			continue
		}
		if length < 0 {
			return nil, errors.New("postgres: invalid DataRow length")
		}
		data, err := cursor.Bytes(int(length))
		if err != nil {
			return nil, err
		}
		value, err := decodeValue(fields[i], data)
		if err != nil {
			return nil, fmt.Errorf("postgres: decode column %q: %w", fields[i].Name, err)
		}
		values[i] = value
	}
	if cursor.Remaining() != 0 {
		return nil, errors.New("postgres: trailing DataRow data")
	}
	return values, nil
}

func decodeValue(field fieldDescription, data []byte) (driver.Value, error) {
	if field.Format == 1 {
		return append([]byte(nil), data...), nil
	}
	text := string(data)
	switch field.TypeOID {
	case 16:
		value, err := strconv.ParseBool(text)
		return value, err
	case 20, 21, 23, 26:
		value, err := strconv.ParseInt(text, 10, 64)
		return value, err
	case 700, 701:
		value, err := strconv.ParseFloat(text, 64)
		return value, err
	case 1700:
		// Preserve NUMERIC precision. database/sql can scan the string into
		// string/[]byte or application decimal types implementing Scanner.
		return text, nil
	case 17:
		if strings.HasPrefix(text, `\x`) {
			value, err := hex.DecodeString(text[2:])
			return value, err
		}
		return append([]byte(nil), data...), nil
	case 1082:
		value, err := time.Parse("2006-01-02", text)
		return value, err
	case 1114:
		value, err := parseTimestamp(text)
		return value, err
	case 1184:
		value, err := parseTimestampTZ(text)
		return value, err
	case 114, 3802:
		return append([]byte(nil), data...), nil
	default:
		return text, nil
	}
}

func parseTimestamp(value string) (time.Time, error) {
	if value == "infinity" || value == "-infinity" {
		return time.Time{}, fmt.Errorf("timestamp %q cannot be represented as time.Time", value)
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid timestamp %q", value)
}

func parseTimestampTZ(value string) (time.Time, error) {
	if value == "infinity" || value == "-infinity" {
		return time.Time{}, fmt.Errorf("timestamptz %q cannot be represented as time.Time", value)
	}
	normalized := strings.Replace(value, " ", "T", 1)
	for _, layout := range []string{
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02T15:04:05.999999999Z0700",
		"2006-01-02T15:04:05Z0700",
		"2006-01-02T15:04:05.999999999Z07",
		"2006-01-02T15:04:05Z07",
	} {
		if parsed, err := time.Parse(layout, normalized); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid timestamptz %q", value)
}

func databaseTypeName(oid uint32) string {
	switch oid {
	case 16:
		return "BOOL"
	case 17:
		return "BYTEA"
	case 20:
		return "INT8"
	case 21:
		return "INT2"
	case 23:
		return "INT4"
	case 25:
		return "TEXT"
	case 26:
		return "OID"
	case 700:
		return "FLOAT4"
	case 701:
		return "FLOAT8"
	case 1042:
		return "BPCHAR"
	case 1043:
		return "VARCHAR"
	case 1082:
		return "DATE"
	case 1114:
		return "TIMESTAMP"
	case 1184:
		return "TIMESTAMPTZ"
	case 1700:
		return "NUMERIC"
	case 2950:
		return "UUID"
	case 114:
		return "JSON"
	case 3802:
		return "JSONB"
	default:
		return fmt.Sprintf("OID_%d", oid)
	}
}

var (
	_ driver.Rows                           = (*rows)(nil)
	_ driver.RowsColumnTypeDatabaseTypeName = (*rows)(nil)
	_ driver.RowsColumnTypeLength           = (*rows)(nil)
	_ driver.RowsColumnTypeNullable         = (*rows)(nil)
	_ driver.RowsNextResultSet              = (*rows)(nil)
)
