package integration

import (
	"context"
	"database/sql"
	"testing"

	postgres "github.com/sevlumen/postgres"
)

func TestReleaseForeignKeyIsolationAndReadOnly(t *testing.T) {
	db := openDatabase(t)
	parent := createTable(t, db, "release_parent", `id bigint PRIMARY KEY`)
	child := createTable(t, db, "release_child", `id bigint PRIMARY KEY, parent_id bigint REFERENCES `+parent+`(id)`)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "INSERT INTO "+child+"(id,parent_id) VALUES($1,$2)", int64(1), int64(999)); err == nil || !postgres.IsForeignKeyViolation(err) {
		t.Fatalf("expected foreign-key violation, got %v", err)
	}
	levels := []struct {
		level sql.IsolationLevel
		want  string
	}{
		{sql.LevelReadUncommitted, "read committed"},
		{sql.LevelReadCommitted, "read committed"},
		{sql.LevelRepeatableRead, "repeatable read"},
		{sql.LevelSerializable, "serializable"},
	}
	for _, test := range levels {
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: test.level})
		if err != nil {
			t.Fatal(err)
		}
		var got string
		if err := tx.QueryRowContext(ctx, `SHOW transaction_isolation`).Scan(&got); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
		if got != test.want {
			_ = tx.Rollback()
			t.Fatalf("isolation=%q want=%q", got, test.want)
		}
		if err := tx.Rollback(); err != nil {
			t.Fatal(err)
		}
	}
	readOnly, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := readOnly.ExecContext(ctx, "INSERT INTO "+parent+"(id) VALUES($1)", int64(3))
	_ = readOnly.Rollback()
	if writeErr == nil || !postgres.IsSQLState(writeErr, "25006") {
		t.Fatalf("expected read-only SQLSTATE 25006, got %v", writeErr)
	}
}

func TestReleaseSerializationFailure(t *testing.T) {
	db := openDatabase(t)
	table := createTable(t, db, "release_serial", `id bigint PRIMARY KEY, value bigint NOT NULL`)
	if _, err := db.Exec("INSERT INTO " + table + "(id,value) VALUES(1,0)"); err != nil {
		t.Fatal(err)
	}
	tx1, err := db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatal(err)
	}
	defer tx1.Rollback()
	tx2, err := db.BeginTx(context.Background(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		t.Fatal(err)
	}
	defer tx2.Rollback()
	var value1, value2 int64
	if err := tx1.QueryRow("SELECT value FROM " + table + " WHERE id=1").Scan(&value1); err != nil {
		t.Fatal(err)
	}
	if err := tx2.QueryRow("SELECT value FROM " + table + " WHERE id=1").Scan(&value2); err != nil {
		t.Fatal(err)
	}
	if _, err := tx1.Exec("UPDATE "+table+" SET value=$1 WHERE id=1", value1+1); err != nil {
		t.Fatal(err)
	}
	if err := tx1.Commit(); err != nil {
		t.Fatal(err)
	}
	_, err = tx2.Exec("UPDATE "+table+" SET value=$1 WHERE id=1", value2+1)
	if err == nil {
		err = tx2.Commit()
	}
	if err == nil || !postgres.IsSerializationFailure(err) {
		t.Fatalf("expected serialization failure, got %v", err)
	}
}
