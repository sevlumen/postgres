package integration

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	postgres "github.com/sevlumen/postgres"
)

func TestSessionPinsBackendAndSupportsParameterizedQueries(t *testing.T) {
	db := openDatabase(t)
	ctx := context.Background()
	session, err := postgres.Acquire(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	var firstPID int
	var secondPID int
	if err := session.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&firstPID); err != nil {
		t.Fatal(err)
	}
	if err := session.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&secondPID); err != nil {
		t.Fatal(err)
	}
	if firstPID == 0 || firstPID != secondPID {
		t.Fatalf("backend pid changed: first=%d second=%d", firstPID, secondPID)
	}

	var value string
	payload := `' OR 1=1; DROP TABLE accounts; --`
	if err := session.QueryRowContext(ctx, `SELECT $1::text`, payload).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != payload {
		t.Fatalf("value=%q want=%q", value, payload)
	}
}

func TestSessionExecScriptSupportsDollarQuotedBodies(t *testing.T) {
	db := openDatabase(t)
	table := createTable(t, db, "script", `id bigint PRIMARY KEY, value text NOT NULL`)
	ctx := context.Background()
	session, err := postgres.Acquire(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	script := fmt.Sprintf(`
INSERT INTO %s(id, value) VALUES (1, 'first');
DO $body$
BEGIN
    INSERT INTO %s(id, value) VALUES (2, 'second;still-data');
END
$body$;
UPDATE %s SET value = value || '-updated' WHERE id = 1;
`, table, table, table)
	result, err := session.ExecScriptContext(ctx, script)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.CommandTags) != 3 {
		t.Fatalf("command tags=%v", result.CommandTags)
	}

	var count int
	if err := session.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("count=%d want=2", count)
	}
}

func TestSessionScriptErrorIsDrainedAndSessionRemainsUsable(t *testing.T) {
	db := openDatabase(t)
	table := createTable(t, db, "script_error", `id bigint PRIMARY KEY`)
	ctx := context.Background()
	session, err := postgres.Acquire(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	_, err = session.ExecScriptContext(ctx, fmt.Sprintf(`
INSERT INTO %s(id) VALUES (1);
SELECT missing_column FROM %s;
`, table, table))
	if err == nil {
		t.Fatal("expected script error")
	}

	var count int
	if err := session.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		t.Fatalf("session was not reusable after script error: %v", err)
	}
	if count != 0 {
		t.Fatalf("implicit script transaction was not rolled back: count=%d", count)
	}
}

func TestSessionScriptInsideManualTransaction(t *testing.T) {
	db := openDatabase(t)
	table := createTable(t, db, "script_tx", `id bigint PRIMARY KEY`)
	ctx := context.Background()
	session, err := postgres.Acquire(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	if _, err := session.ExecContext(ctx, `BEGIN`); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecScriptContext(ctx, fmt.Sprintf(`
INSERT INTO %s(id) VALUES (1);
INSERT INTO %s(id) VALUES (2);
`, table, table)); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecContext(ctx, `COMMIT`); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("count=%d want=2", count)
	}

	if _, err := session.ExecContext(ctx, `BEGIN`); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecScriptContext(ctx, fmt.Sprintf(`DELETE FROM %s`, table)); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ExecContext(ctx, `ROLLBACK`); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("rollback count=%d want=2", count)
	}
}

func TestSessionScriptCancellationDiscardsBackendAndPoolRemainsUsable(t *testing.T) {
	db := openDatabase(t)
	session, err := postgres.Acquire(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err = session.ExecScriptContext(ctx, `SELECT pg_sleep(5); SELECT 1`)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
	if err := session.QueryRowContext(context.Background(), `SELECT 1`).Scan(new(int)); err == nil {
		t.Fatal("discarded session remained usable")
	}

	var value int
	if err := db.QueryRowContext(context.Background(), `SELECT 42`).Scan(&value); err != nil {
		t.Fatalf("pool was not reusable: %v", err)
	}
	if value != 42 {
		t.Fatalf("value=%d want=42", value)
	}
}

func TestSessionDiscardIsIdempotent(t *testing.T) {
	db := openDatabase(t)
	session, err := postgres.Acquire(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Discard(); err != nil {
		t.Fatal(err)
	}
	if err := session.Discard(); err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	var value int
	if err := db.QueryRowContext(context.Background(), `SELECT 7`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != 7 {
		t.Fatalf("value=%d want=7", value)
	}
}
