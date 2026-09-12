// Package storage owns every SQL statement and the SQLite connection policy.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// dsnPragmas is the mandatory modernc.org/sqlite DSN pragma suffix. modernc
// silently ignores the mattn-style `_journal_mode=` / `_busy_timeout=` keys, so
// the `_pragma=name(value)` form is the only one that takes effect (design R4).
const dsnPragmas = "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"

// DB wraps the raw connection pool. The pool stays unexported so later slices
// can funnel every query through actor-guarded read/write helpers.
type DB struct {
	raw *sql.DB
}

// Open creates the data directory, opens SQLite with the mandatory pragmas,
// constrains the pool to a single connection, hard-fails unless the pragmas
// actually applied, and then runs pending migrations.
func Open(ctx context.Context, path string) (*DB, error) {
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create data directory: %w", err)
		}
	}

	raw, err := sql.Open("sqlite", "file:"+path+"?"+dsnPragmas)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite writes serialize on one connection; WAL plus a busy timeout keeps
	// concurrent readers safe at this scale.
	raw.SetMaxOpenConns(1)
	raw.SetMaxIdleConns(1)
	raw.SetConnMaxLifetime(0)

	db := &DB{raw: raw}
	if err := db.assertPragmas(ctx); err != nil {
		_ = raw.Close()
		return nil, err
	}
	if err := db.Migrate(ctx); err != nil {
		_ = raw.Close()
		return nil, err
	}
	return db, nil
}

// Close releases the underlying connection pool.
func (d *DB) Close() error {
	if d == nil || d.raw == nil {
		return nil
	}
	return d.raw.Close()
}

// Health reports the live pragma state that /healthz exposes.
func (d *DB) Health(ctx context.Context) (journalMode string, foreignKeys int, err error) {
	if err := d.raw.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		return "", 0, fmt.Errorf("read journal_mode: %w", err)
	}
	if err := d.raw.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		return "", 0, fmt.Errorf("read foreign_keys: %w", err)
	}
	return journalMode, foreignKeys, nil
}

// assertPragmas hard-fails the boot when WAL or foreign keys did not apply.
func (d *DB) assertPragmas(ctx context.Context) error {
	journal, fk, err := d.Health(ctx)
	if err != nil {
		return fmt.Errorf("assert pragmas: %w", err)
	}
	if !strings.EqualFold(journal, "wal") {
		return fmt.Errorf("assert pragmas: journal_mode = %q, want wal", journal)
	}
	if fk != 1 {
		return fmt.Errorf("assert pragmas: foreign_keys = %d, want 1", fk)
	}
	return nil
}
