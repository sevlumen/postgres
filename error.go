package postgres

import (
	"errors"
	"fmt"
)

// Error is a structured PostgreSQL ErrorResponse.
type Error struct {
	Severity         string
	SeverityNonLocal string
	Code             string
	Message          string
	Detail           string
	Hint             string
	Position         string
	InternalPosition string
	InternalQuery    string
	Where            string
	Schema           string
	Table            string
	Column           string
	DataType         string
	Constraint       string
	File             string
	Line             string
	Routine          string
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Code == "" {
		return e.Message
	}
	return fmt.Sprintf("postgres: %s (SQLSTATE %s)", e.Message, e.Code)
}

// SQLState returns the five-character PostgreSQL SQLSTATE code.
func (e *Error) SQLState() string { return e.Code }

func IsSQLState(err error, code string) bool {
	var pgErr *Error
	return errors.As(err, &pgErr) && pgErr.Code == code
}

func IsUniqueViolation(err error) bool      { return IsSQLState(err, "23505") }
func IsForeignKeyViolation(err error) bool  { return IsSQLState(err, "23503") }
func IsSerializationFailure(err error) bool { return IsSQLState(err, "40001") }
func IsDeadlockDetected(err error) bool     { return IsSQLState(err, "40P01") }
