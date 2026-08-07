package postgres

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
)

func TestNormalizeValueDereferencesPointers(t *testing.T) {
	t.Parallel()
	text := "member@example.test"
	number := int32(42)
	pointer := &text

	tests := []struct {
		name  string
		value any
		want  any
	}{
		{name: "string", value: &text, want: text},
		{name: "integer", value: &number, want: int64(42)},
		{name: "nested", value: &pointer, want: text},
		{name: "valuer", value: sql.NullString{String: "value", Valid: true}, want: "value"},
		{name: "null valuer", value: sql.NullString{}, want: nil},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeValue(test.value)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("normalizeValue(%T) = %#v, want %#v", test.value, got, test.want)
			}
		})
	}
}

func TestNormalizeValueTreatsNilPointersAsNull(t *testing.T) {
	t.Parallel()
	var text *string
	var nested **string
	for _, value := range []any{text, nested} {
		got, err := normalizeValue(value)
		if err != nil {
			t.Fatal(err)
		}
		if got != nil {
			t.Fatalf("normalizeValue(%T) = %#v, want nil", value, got)
		}
	}
}

func TestEncodeArgumentsSupportsPointerValues(t *testing.T) {
	t.Parallel()
	text := "pointer-value"
	var nullText *string
	encoded, err := encodeArguments([]driver.NamedValue{
		{Ordinal: 1, Value: &text},
		{Ordinal: 2, Value: nullText},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(encoded.values[0]); got != text || encoded.nulls[0] {
		t.Fatalf("first argument = %q null=%t", got, encoded.nulls[0])
	}
	if encoded.values[1] != nil || !encoded.nulls[1] {
		t.Fatalf("second argument = %q null=%t", encoded.values[1], encoded.nulls[1])
	}
}

var errFailingValuer = errors.New("valuer failed")

type failingValuer struct{}

func (failingValuer) Value() (driver.Value, error) {
	return nil, errFailingValuer
}

func TestNormalizeValuePreservesValuerErrors(t *testing.T) {
	t.Parallel()
	_, err := normalizeValue(failingValuer{})
	if !errors.Is(err, errFailingValuer) {
		t.Fatalf("error = %v, want wrapped valuer error", err)
	}
}
