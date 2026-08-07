package integration

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestIdentityRefreshRotationDogfood(t *testing.T) {
	db := openDatabase(t)
	table := createTable(t, db, "refresh", `token_hash text PRIMARY KEY, consumed boolean NOT NULL DEFAULT false`)
	if _, err := db.Exec("INSERT INTO "+table+"(token_hash) VALUES($1)", "old-token"); err != nil {
		t.Fatal(err)
	}
	const contenders = 16
	var successes atomic.Int32
	var wait sync.WaitGroup
	errorsCh := make(chan error, contenders)
	for i := 0; i < contenders; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			tx, err := db.BeginTx(context.Background(), nil)
			if err != nil {
				errorsCh <- err
				return
			}
			defer tx.Rollback()
			result, err := tx.Exec("UPDATE "+table+" SET consumed=true WHERE token_hash=$1 AND consumed=false", "old-token")
			if err != nil {
				errorsCh <- err
				return
			}
			rows, err := result.RowsAffected()
			if err != nil {
				errorsCh <- err
				return
			}
			if rows == 0 {
				return
			}
			if _, err := tx.Exec("INSERT INTO "+table+"(token_hash) VALUES($1)", fmt.Sprintf("replacement-%d", index)); err != nil {
				errorsCh <- err
				return
			}
			if err := tx.Commit(); err != nil {
				errorsCh <- err
				return
			}
			successes.Add(1)
		}(i)
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Error(err)
	}
	if got := successes.Load(); got != 1 {
		t.Fatalf("successful rotations=%d, want 1", got)
	}
}
