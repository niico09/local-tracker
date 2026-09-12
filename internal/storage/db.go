// Package storage owns every SQL statement and the SQLite connection policy.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"local-tracker/internal/domain"

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

// Repos bundles the domain port implementations on one connection.
type Repos struct {
	Users      *UserRepo
	Sessions   *SessionRepo
	Items      *ItemRepo
	Goals      *GoalRepo
	Membership *MembershipRepo
}

// NewRepos wires the repositories.
func NewRepos(db *DB) *Repos {
	return &Repos{
		Users:      &UserRepo{db: db},
		Sessions:   &SessionRepo{db: db},
		Items:      &ItemRepo{db: db},
		Goals:      &GoalRepo{db: db},
		Membership: &MembershipRepo{db: db},
	}
}

// read runs a query only when an acting user is present. No actor => no SQL.
func (d *DB) read(ctx context.Context, a domain.Actor, q string, args ...any) (*sql.Rows, error) {
	if err := a.Require(); err != nil {
		return nil, err
	}
	return d.raw.QueryContext(ctx, q, args...)
}

// write authorizes a mutation before touching SQL.
func (d *DB) write(ctx context.Context, a domain.Actor, s domain.Subject, act domain.Action, q string, args ...any) (sql.Result, error) {
	if err := domain.Authorize(a, act, s); err != nil {
		return nil, err
	}
	return d.raw.ExecContext(ctx, q, args...)
}

// writeTx is the transactional form of write, used inside atomic use cases.
func writeTx(ctx context.Context, tx *sql.Tx, a domain.Actor, s domain.Subject, act domain.Action, q string, args ...any) (sql.Result, error) {
	if err := domain.Authorize(a, act, s); err != nil {
		return nil, err
	}
	return tx.ExecContext(ctx, q, args...)
}

// count runs a scalar COUNT query through the actor guard.
func (d *DB) count(ctx context.Context, a domain.Actor, q string, args ...any) (int, error) {
	rows, err := d.read(ctx, a, q, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, rows.Err()
	}
	var n int
	if err := rows.Scan(&n); err != nil {
		return 0, err
	}
	return n, rows.Err()
}

// subjected is any row that can report its ownership facts to Authorize.
type subjected interface{ Subject() domain.Subject }

// readAll post-filters every returned row through Authorize(ActionView), so a
// forgotten WHERE can never leak a hidden row.
func readAll[T subjected](ctx context.Context, d *DB, a domain.Actor, q string, args []any, scan func(*sql.Rows) (T, error)) ([]T, error) {
	rows, err := d.read(ctx, a, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		if domain.Authorize(a, domain.ActionView, v.Subject()) != nil {
			continue
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// readOne is readAll for a single row; a denied row surfaces as ErrForbidden
// and an absent one as ErrNotFound.
func readOne[T subjected](ctx context.Context, d *DB, a domain.Actor, q string, args []any, scan func(*sql.Rows) (T, error)) (T, error) {
	var zero T
	rows, err := d.read(ctx, a, q, args...)
	if err != nil {
		return zero, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return zero, err
		}
		return zero, domain.ErrNotFound
	}
	v, err := scan(rows)
	if err != nil {
		return zero, err
	}
	if err := domain.Authorize(a, domain.ActionView, v.Subject()); err != nil {
		return zero, err
	}
	return v, rows.Err()
}

const timeLayout = time.RFC3339

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseTime(s string) time.Time {
	t, _ := time.Parse(timeLayout, s)
	return t
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
