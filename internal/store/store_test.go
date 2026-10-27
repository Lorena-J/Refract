package store

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestOpenMigratesOnceAndReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "refract.db")
	for range 2 { // second open must be a no-op, not a re-run
		db, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var v int
		if err := db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		if v != len(migrations) {
			t.Fatalf("user_version = %d, want %d", v, len(migrations))
		}
		db.Close()
	}
}

// Many goroutines reading and writing at once: no "database is locked", and a
// query made while iterating another's rows does not block (it did with a
// single pooled connection).
func TestConcurrentAccess(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 200)
	for i := range 20 {
		wg.Go(func() {
			for j := range 10 {
				k := fmt.Sprintf("k%d-%d", i, j)
				if err := SetSetting(ctx, db, k, "v"); err != nil {
					errs <- err
				}
				if _, err := GetSetting(ctx, db, k); err != nil {
					errs <- err
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		rows, err := db.QueryContext(ctx, `SELECT key FROM settings LIMIT 3`)
		if err != nil {
			done <- err
			return
		}
		defer rows.Close()
		for rows.Next() {
			var k string
			rows.Scan(&k)
			if _, err := GetSetting(ctx, db, k); err != nil { // nested query
				done <- err
				return
			}
		}
		done <- rows.Err()
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nested query blocked")
	}
}
