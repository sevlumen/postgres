package postgres

import (
	"errors"
	"testing"
)

func TestSQLStateClassification(t *testing.T) {
	err := &Error{Code: "23505", Message: "duplicate key"}
	if !IsUniqueViolation(err) {
		t.Fatal("expected unique violation")
	}
	if IsForeignKeyViolation(err) {
		t.Fatal("unexpected foreign key classification")
	}
	var target *Error
	if !errors.As(err, &target) || target.SQLState() != "23505" {
		t.Fatal("structured error unavailable")
	}
}
