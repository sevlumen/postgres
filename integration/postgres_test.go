package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	postgres "github.com/sevlumen/postgres"
)

func openDatabase(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("SEVLUMEN_POSTGRES_TEST_DSN")
	if dsn == "" {
		t.Skip("SEVLUMEN_POSTGRES_TEST_DSN is not set")
	}
	db, err := postgres.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestQueryExecTypesAndPreparedStatements(t *testing.T) {
	db := openDatabase(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TEMP TABLE driver_values (
		id bigint PRIMARY KEY,
		name text NOT NULL,
		active boolean NOT NULL,
		payload bytea,
		created_at timestamptz NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	created := time.Now().UTC().Truncate(time.Microsecond)
	payload := []byte{0, 1, 2, 255}
	statement, err := db.PrepareContext(ctx, `INSERT INTO driver_values(id,name,active,payload,created_at) VALUES($1,$2,$3,$4,$5)`)
	if err != nil {
		t.Fatal(err)
	}
	defer statement.Close()
	result, err := statement.ExecContext(ctx, int64(1), "identity", true, payload, created)
	if err != nil {
		t.Fatal(err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil || rowsAffected != 1 {
		t.Fatalf("rows affected=%d err=%v", rowsAffected, err)
	}
	var id int64
	var name string
	var active bool
	var returnedPayload []byte
	var returnedCreated time.Time
	if err := db.QueryRowContext(ctx, `SELECT id,name,active,payload,created_at FROM driver_values WHERE id=$1`, int64(1)).Scan(&id, &name, &active, &returnedPayload, &returnedCreated); err != nil {
		t.Fatal(err)
	}
	if id != 1 || name != "identity" || !active || string(returnedPayload) != string(payload) || !returnedCreated.Equal(created) {
		t.Fatalf("unexpected row: id=%d name=%q active=%v payload=%v created=%v", id, name, active, returnedPayload, returnedCreated)
	}
}

func TestTransactionCommitRollbackAndSQLState(t *testing.T) {
	db := openDatabase(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `CREATE TEMP TABLE tx_values (id bigint PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tx_values(id) VALUES($1)`, int64(1)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	tx, err = db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tx_values(id) VALUES($1)`, int64(2)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM tx_values`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("count=%d, want 1", count)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO tx_values(id) VALUES($1)`, int64(1))
	if err == nil || !postgres.IsUniqueViolation(err) {
		t.Fatalf("expected unique violation, got %v", err)
	}
}

func TestCancellationKeepsPoolUsable(t *testing.T) {
	db := openDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := db.ExecContext(ctx, `SELECT pg_sleep(5)`)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline, got %v", err)
	}
	var value int
	if err := db.QueryRowContext(context.Background(), `SELECT 42`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != 42 {
		t.Fatalf("value=%d", value)
	}
}

func TestConcurrentPoolUse(t *testing.T) {
	db := openDatabase(t)
	const workers = 16
	var wait sync.WaitGroup
	errorsCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			var value int
			if err := db.QueryRowContext(context.Background(), `SELECT $1::bigint`, int64(index)).Scan(&value); err != nil {
				errorsCh <- err
				return
			}
			if value != index {
				errorsCh <- fmt.Errorf("value=%d want=%d", value, index)
			}
		}(i)
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Error(err)
	}
}
