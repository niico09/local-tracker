package storage

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// Migrate ensures the migration ledger exists and applies every unapplied
// migration in filename order, each inside its own transaction. Re-running it
// applies nothing, so it is safe on every boot.
func (d *DB) Migrate(ctx context.Context) error {
	if _, err := d.raw.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	names, err := fs.Glob(migrationFS, "migrations/*.sql")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	sort.Strings(names)

	for _, name := range names {
		version, err := migrationVersion(name)
		if err != nil {
			return err
		}

		var applied int
		if err := d.raw.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
		if applied > 0 {
			continue
		}
		if err := d.applyMigration(ctx, name, version); err != nil {
			return err
		}
	}
	return nil
}

// applyMigration runs one migration and records its version in a single tx.
func (d *DB) applyMigration(ctx context.Context, name string, version int) error {
	body, err := migrationFS.ReadFile(name)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", name, err)
	}

	tx, err := d.raw.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx, string(body)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("apply migration %d: %w", version, err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)",
		version, time.Now().UTC().Format(time.RFC3339)); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("record migration %d: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %d: %w", version, err)
	}
	return nil
}

// migrationVersion parses the leading integer of a migration filename.
func migrationVersion(name string) (int, error) {
	base := strings.TrimSuffix(strings.TrimPrefix(name, "migrations/"), ".sql")
	prefix, _, _ := strings.Cut(base, "_")
	version, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, fmt.Errorf("migration %q: version prefix %q is not an integer", name, prefix)
	}
	return version, nil
}
