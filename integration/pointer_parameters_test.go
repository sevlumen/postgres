package integration

import (
	"context"
	"database/sql"
	"testing"
)

func TestPointerAndValuerParameters(t *testing.T) {
	db := openDatabase(t)
	ctx := context.Background()

	text := "pointer-value"
	var returned string
	if err := db.QueryRowContext(ctx, `SELECT $1::text`, &text).Scan(&returned); err != nil {
		t.Fatal(err)
	}
	if returned != text {
		t.Fatalf("returned=%q want=%q", returned, text)
	}

	var nullText *string
	var nullable sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT $1::text`, nullText).Scan(&nullable); err != nil {
		t.Fatal(err)
	}
	if nullable.Valid {
		t.Fatalf("nil pointer returned a value: %#v", nullable)
	}

	valuer := sql.NullString{String: "valuer-value", Valid: true}
	if err := db.QueryRowContext(ctx, `SELECT $1::text`, valuer).Scan(&returned); err != nil {
		t.Fatal(err)
	}
	if returned != valuer.String {
		t.Fatalf("valuer returned=%q want=%q", returned, valuer.String)
	}
}
