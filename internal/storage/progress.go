package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"local-tracker/internal/domain"
)

// ProgressRepo implements domain.ProgressRepository. It owns the progress table
// and the NULL-safe upsert keyed by the expression index
// ux_progress_item_owner(item_id, COALESCE(owner_user_id, 0)).
type ProgressRepo struct{ db *DB }

// progressColumns is the shared select list for every tilt read.
const progressColumns = "id, item_id, owner_user_id, done, start_date, end_date"

// upsertProgressSQL is the NULL-safe upsert. The conflict target names the same
// expression as the unique index, so the shared (NULL) tilt and each personal
// tilt stay unique without a sentinel owner id.
const upsertProgressSQL = "INSERT INTO progress(item_id, owner_user_id, done, start_date, end_date, updated_at) VALUES (?,?,?,?,?,?) " +
	"ON CONFLICT(item_id, COALESCE(owner_user_id, 0)) DO UPDATE SET " +
	"done=excluded.done, start_date=excluded.start_date, end_date=excluded.end_date, updated_at=excluded.updated_at"

// tiltRow pairs a progress row with the ownership facts of its enclosing
// resource (the goal inside a goal, the item standalone) so the shared read
// helpers authorize through the single access rule: a granted partner reads a
// personal goal's tilt read-only, which the tilt owner alone could not express.
type tiltRow struct {
	domain.Progress
	subject domain.Subject
}

// Subject reports the enclosing resource's ownership facts.
func (t tiltRow) Subject() domain.Subject { return t.subject }

// Tilt resolves and returns the tilt for one item. goalID 0 selects the
// standalone tilt from the item owner (G5); a positive goalID selects the Rule 1
// tilt from the goal owner and requires the item to be a member. A missing row
// is a pending tilt carrying the resolved owner, never an error.
func (r *ProgressRepo) Tilt(ctx context.Context, a domain.Actor, goalID domain.GoalID, itemID domain.ItemID) (domain.Progress, error) {
	owner, subject, err := r.resolve(ctx, a, goalID, itemID, domain.ActionView)
	if err != nil {
		return domain.Progress{}, err
	}
	p, err := r.getTilt(ctx, a, itemID, owner, subject)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Progress{ItemID: itemID, OwnerUserID: owner}, nil
	}
	return p, err
}

// SetTilt upserts one tilt. The owner is always resolved server-side, so a
// client can never write another profile's tilt; a granted partner is refused
// because modify permission is required on the enclosing goal or item.
func (r *ProgressRepo) SetTilt(ctx context.Context, a domain.Actor, goalID domain.GoalID, p domain.Progress) (domain.Progress, error) {
	if p.ItemID <= 0 {
		return domain.Progress{}, fmt.Errorf("%w: item id is required", domain.ErrValidation)
	}
	if err := domain.ValidateDates(p.StartDate, p.EndDate); err != nil {
		return domain.Progress{}, err
	}
	owner, subject, err := r.resolve(ctx, a, goalID, p.ItemID, domain.ActionModify)
	if err != nil {
		return domain.Progress{}, err
	}
	if _, err := r.db.write(ctx, a, subject, domain.ActionModify, upsertProgressSQL,
		int64(p.ItemID), nullUser(owner), boolInt(p.Done), nullDate(p.StartDate), nullDate(p.EndDate),
		formatTime(time.Now().UTC())); err != nil {
		return domain.Progress{}, err
	}
	return r.getTilt(ctx, a, p.ItemID, owner, subject)
}

// GoalRollup counts the goal's member items and the done tilts that belong to
// the goal owner's tilt, then applies the target-else-member-count denominator
// through domain.Rollup. The goal is authorized first, so a hidden goal returns
// ErrForbidden and a granted partner reads the rollup read-only.
func (r *ProgressRepo) GoalRollup(ctx context.Context, a domain.Actor, goalID domain.GoalID) (done, total int, err error) {
	if goalID <= 0 {
		return 0, 0, fmt.Errorf("%w: goal id is required", domain.ErrValidation)
	}
	goal, err := readOne(ctx, r.db, a,
		"SELECT "+goalColumns+" FROM goals WHERE id = ?", []any{int64(goalID)}, scanGoal)
	if err != nil {
		return 0, 0, err
	}
	rows, err := r.db.read(ctx, a,
		"SELECT COUNT(*), COALESCE(SUM(CASE WHEN p.done = 1 THEN 1 ELSE 0 END), 0) "+
			"FROM goal_items gi "+
			"JOIN goals g ON g.id = gi.goal_id "+
			"LEFT JOIN progress p ON p.item_id = gi.item_id AND p.owner_user_id IS g.owner_user_id "+
			"WHERE gi.goal_id = ?", int64(goalID))
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()

	var memberCount, memberDone int
	if rows.Next() {
		if err := rows.Scan(&memberCount, &memberDone); err != nil {
			return 0, 0, err
		}
	}
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	done, total = domain.Rollup(goal.Target, memberDone, memberCount)
	return done, total, nil
}

// resolve loads the enclosing resource, authorizes the requested action, and
// derives the tilt owner: the goal owner inside a goal (Rule 1), the item owner
// standalone (G5). A goal tilt additionally requires the item to be a member of
// that goal, so a tilt can never be written against an unrelated item.
func (r *ProgressRepo) resolve(ctx context.Context, a domain.Actor, goalID domain.GoalID, itemID domain.ItemID, act domain.Action) (domain.UserID, domain.Subject, error) {
	if goalID > 0 {
		goal, err := readOne(ctx, r.db, a,
			"SELECT "+goalColumns+" FROM goals WHERE id = ?", []any{int64(goalID)}, scanGoal)
		if err != nil {
			return 0, domain.Subject{}, err
		}
		if act == domain.ActionModify {
			if err := domain.Authorize(a, act, goal.Subject()); err != nil {
				return 0, domain.Subject{}, err
			}
		}
		member, err := r.db.count(ctx, a,
			"SELECT COUNT(*) FROM goal_items WHERE goal_id = ? AND item_id = ?", int64(goalID), int64(itemID))
		if err != nil {
			return 0, domain.Subject{}, err
		}
		if member == 0 {
			return 0, domain.Subject{}, domain.ErrNotFound
		}
		return domain.TiltOwnerForGoal(goal), goal.Subject(), nil
	}

	item, err := readOne(ctx, r.db, a,
		"SELECT "+itemColumns+" FROM items WHERE id = ?", []any{int64(itemID)}, scanItem)
	if err != nil {
		return 0, domain.Subject{}, err
	}
	if act == domain.ActionModify {
		if err := domain.Authorize(a, act, item.Subject()); err != nil {
			return 0, domain.Subject{}, err
		}
	}
	return domain.TiltOwnerForItem(item), item.Subject(), nil
}

// getTilt loads one stored tilt, authorizing through the enclosing subject.
func (r *ProgressRepo) getTilt(ctx context.Context, a domain.Actor, itemID domain.ItemID, owner domain.UserID, subject domain.Subject) (domain.Progress, error) {
	scan := func(rows *sql.Rows) (tiltRow, error) { return scanTilt(rows, subject) }
	row, err := readOne(ctx, r.db, a,
		"SELECT "+progressColumns+" FROM progress WHERE item_id = ? AND owner_user_id IS ?",
		[]any{int64(itemID), nullUser(owner)}, scan)
	if err != nil {
		return domain.Progress{}, err
	}
	return row.Progress, nil
}

// scanTilt reads one progress row. NULL dates stay nil; a NULL owner is the
// shared couple tilt (0).
func scanTilt(rows *sql.Rows, subject domain.Subject) (tiltRow, error) {
	var (
		p          domain.Progress
		owner      sql.NullInt64
		done       sql.NullInt64
		start, end sql.NullString
	)
	if err := rows.Scan(&p.ID, &p.ItemID, &owner, &done, &start, &end); err != nil {
		return tiltRow{}, err
	}
	if owner.Valid {
		p.OwnerUserID = domain.UserID(owner.Int64)
	}
	p.Done = done.Valid && done.Int64 == 1
	if start.Valid {
		d := parseDate(start.String)
		p.StartDate = &d
	}
	if end.Valid {
		d := parseDate(end.String)
		p.EndDate = &d
	}
	return tiltRow{Progress: p, subject: subject}, nil
}

// nullDate maps an unset date to SQL NULL.
func nullDate(d *domain.Date) any {
	if d == nil {
		return nil
	}
	return d.String()
}

// boolInt maps a bool to SQLite's 0/1 storage.
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
