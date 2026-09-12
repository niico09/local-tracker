package storage

import (
	"context"
	"database/sql"
	"time"

	"local-tracker/internal/domain"
)

// SeedRepo implements domain.SeedRepository. It owns the idempotent Top 100
// application and nothing else.
type SeedRepo struct{ db *DB }

const (
	top100ExternalKey = "top100"
	top100Title       = "Top 100"
	top100Target      = 100
)

// Seed applies the Top 100 list inside one transaction. Every insert relies on
// a schema constraint for idempotency (UNIQUE(items.external_id),
// UNIQUE(goals.external_key), goal_items primary key), so a second run inserts
// nothing and reports the same counts. The seeded goal is a couple goal
// (owner NULL) with target 100, i.e. one shared advance for both partners.
func (r *SeedRepo) Seed(ctx context.Context, a domain.Actor, entries []domain.SeedEntry) (domain.SeedResult, error) {
	var result domain.SeedResult
	if err := a.Require(); err != nil {
		return result, err
	}
	tx, err := r.db.raw.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() { _ = tx.Rollback() }()

	now := formatTime(time.Now().UTC())
	goalID, created, err := ensureTop100Goal(ctx, tx, a, now)
	if err != nil {
		return domain.SeedResult{}, err
	}
	result.GoalCreated = created

	for _, e := range entries {
		res, err := writeTx(ctx, tx, a, domain.Subject{}, domain.ActionModify,
			"INSERT INTO items(title, kind, year, external_id, cover_path, owner_user_id, created_at, updated_at) "+
				"VALUES (?,?,?,?,NULL,NULL,?,?) ON CONFLICT(external_id) DO NOTHING",
			e.Title, string(e.Kind), nullInt(e.Year), e.ExternalID, now, now)
		if err != nil {
			return domain.SeedResult{}, err
		}
		inserted, err := res.RowsAffected()
		if err != nil {
			return domain.SeedResult{}, err
		}
		if inserted == 0 {
			result.ItemsKept++
		} else {
			result.ItemsInserted++
		}

		// INSERT OR IGNORE ... SELECT resolves the item id by external_id and
		// does nothing when the membership already exists.
		member, err := writeTx(ctx, tx, a, domain.Subject{}, domain.ActionModify,
			"INSERT OR IGNORE INTO goal_items(goal_id, item_id, added_at) "+
				"SELECT ?, id, ? FROM items WHERE external_id = ?",
			int64(goalID), now, e.ExternalID)
		if err != nil {
			return domain.SeedResult{}, err
		}
		added, err := member.RowsAffected()
		if err != nil {
			return domain.SeedResult{}, err
		}
		result.MembershipsAdded += int(added)
	}

	if err := tx.Commit(); err != nil {
		return domain.SeedResult{}, err
	}
	return result, nil
}

// ensureTop100Goal returns the seeded couple goal's id, inserting it when the
// external key is still absent. An existing goal is adopted unchanged so a
// customized title or target is never clobbered by a later seed run.
func ensureTop100Goal(ctx context.Context, tx *sql.Tx, a domain.Actor, now string) (domain.GoalID, bool, error) {
	res, err := writeTx(ctx, tx, a, domain.Subject{}, domain.ActionModify,
		"INSERT INTO goals(title, owner_user_id, target, visibility, external_key, created_at, updated_at) "+
			"VALUES (?,NULL,?,'private',?,?,?) ON CONFLICT(external_key) DO NOTHING",
		top100Title, top100Target, top100ExternalKey, now, now)
	if err != nil {
		return 0, false, err
	}
	inserted, err := res.RowsAffected()
	if err != nil {
		return 0, false, err
	}
	id, err := scalarIntTx(ctx, tx, a, "SELECT id FROM goals WHERE external_key = ?", top100ExternalKey)
	if err != nil {
		return 0, false, err
	}
	return domain.GoalID(id), inserted > 0, nil
}

// scalarIntTx is the transactional scalar read used by the seed transaction,
// keeping the actor guard around every statement. Seed is the only caller.
func scalarIntTx(ctx context.Context, tx *sql.Tx, a domain.Actor, q string, args ...any) (int64, error) {
	if err := a.Require(); err != nil {
		return 0, err
	}
	var n int64
	if err := tx.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
