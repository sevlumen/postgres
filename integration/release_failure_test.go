package integration

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestEarlyRowsCloseAndServerDisconnectDiscardConnections(t *testing.T) {
	db := openDatabase(t)
	rows, err := db.QueryContext(context.Background(), `SELECT generate_series(1,100000)`)
	if err != nil {
		t.Fatal(err)
	}
	if !rows.Next() {
		t.Fatalf("expected first row: %v", rows.Err())
	}
	var first int
	if err := rows.Scan(&first); err != nil || first != 1 {
		t.Fatalf("first=%d err=%v", first, err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	assertQueryValue(t, db, 43)

	victim, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer victim.Close()
	var pid int
	if err := victim.QueryRowContext(context.Background(), `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(100 * time.Millisecond)
		_, _ = db.Exec(`SELECT pg_terminate_backend($1)`, int64(pid))
	}()
	if _, err := victim.ExecContext(context.Background(), `SELECT pg_sleep(5)`); err == nil {
		t.Fatal("expected query disconnect error")
	}
	assertQueryValue(t, db, 44)
}

func TestManualCancellationReturnsContextError(t *testing.T) {
	db := openDatabase(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	_, err := db.ExecContext(ctx, `SELECT pg_sleep(5)`)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	assertQueryValue(t, db, 45)
}

func assertQueryValue(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var value int
	if err := db.QueryRowContext(context.Background(), `SELECT $1::bigint`, int64(want)).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != want {
		t.Fatalf("value=%d want=%d", value, want)
	}
}
