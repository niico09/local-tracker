package storage

import (
	"context"
	"path/filepath"
	"testing"
)

// openTestDB opens a throwaway database under t.TempDir and closes it on cleanup.
func openTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "tracker.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOpenAssertsPragmas(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	journal, fk, err := db.Health(ctx)
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if journal != "wal" {
		t.Errorf("journal_mode = %q, want wal", journal)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, want 1", fk)
	}

	var busyTimeout int
	if err := db.raw.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
		t.Fatalf("read busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000", busyTimeout)
	}
}
