package storage

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"local-tracker/internal/domain"
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

// TestWritesSerializeOnOneConnection proves the single-connection scenario:
// bounded concurrent writes all succeed (no SQLITE_BUSY), the pool caps at one
// connection, and the final state is correct.
func TestWritesSerializeOnOneConnection(t *testing.T) {
	db := openTestDB(t)
	if got := db.raw.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("MaxOpenConnections = %d, want 1", got)
	}
	repos := NewRepos(db)
	seedUsers(t, repos)
	const writers = 16
	actor, ctx := domain.NewActor(1), context.Background()
	errs := make([]error, writers)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, errs[n] = repos.Items.Create(ctx, actor, sampleItem("Same"))
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent Create: %v", err)
		}
	}
	if list, err := repos.Items.List(ctx, actor); err != nil || len(list) != writers {
		t.Fatalf("items = %d, %v; want %d", len(list), err, writers)
	}
}
