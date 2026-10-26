package store

import (
	"path/filepath"
	"testing"
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
