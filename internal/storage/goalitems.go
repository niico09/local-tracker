package storage

import (
	"context"
	"database/sql"
	"time"

	"local-tracker/internal/domain"
)

// MembershipRepo implements domain.MembershipRepository. It owns the goal_items
// link table and the NULL-safe Rule 1 tilt join; it never touches progress
// writes (slice S5).
type MembershipRepo struct{ db *DB }

// membershipColumns selects each member item together with the tilt row that
// belongs to the enclosing goal. The join compares owners with `IS` so NULL
// (shared) matches NULL and a shared tilt is never confused with a personal one.
const membershipColumns = "i.id, i.title, i.kind, i.year, i.external_id, i.cover_path, i.owner_user_id, i.created_at, i.updated_at, " +
	"p.id, p.done, p.start_date, p.end_date"

// membershipJoin is the shared FROM ... LEFT JOIN clause; the `IS` comparison is
// what makes Rule 1 resolve the goal owner's tilt (I4).
const membershipJoin = "FROM goal_items gi " +
	"JOIN items i ON i.id = gi.item_id " +
	"JOIN goals g ON g.id = gi.goal_id " +
	"LEFT JOIN progress p ON p.item_id = i.id AND p.owner_user_id IS g.owner_user_id"

// Add links an item into a goal. It requires modify permission on the enclosing
// goal and visibility of the item; a duplicate link is a no-op so a double
// submit stays idempotent.
func (r *MembershipRepo) Add(ctx context.Context, a domain.Actor, goalID domain.GoalID, itemID domain.ItemID) error {
	goal, err := goalForModify(ctx, r.db, a, goalID)
	if err != nil {
		return err
	}
	if _, err := readOne(ctx, r.db, a, "SELECT "+itemColumns+" FROM items WHERE id = ?", []any{int64(itemID)}, scanItem); err != nil {
		return err
	}
	_, err = r.db.write(ctx, a, goal.Subject(), domain.ActionModify,
		"INSERT INTO goal_items(goal_id, item_id, added_at) VALUES (?,?,?) ON CONFLICT(goal_id, item_id) DO NOTHING",
		int64(goalID), int64(itemID), formatTime(time.Now().UTC()))
	return err
}

// Remove deletes only the membership link. The item and every progress row it
// owns survive: goal_items cascades from items/goals, never the reverse.
func (r *MembershipRepo) Remove(ctx context.Context, a domain.Actor, goalID domain.GoalID, itemID domain.ItemID) error {
	goal, err := goalForModify(ctx, r.db, a, goalID)
	if err != nil {
		return err
	}
	_, err = r.db.write(ctx, a, goal.Subject(), domain.ActionModify,
		"DELETE FROM goal_items WHERE goal_id = ? AND item_id = ?", int64(goalID), int64(itemID))
	return err
}

// ListMembers returns the items in one goal with their Rule 1 tilt owner and
// resolved progress row. The goal itself is authorized first (view), so a
// hidden goal returns ErrForbidden and a granted partner may read its members.
func (r *MembershipRepo) ListMembers(ctx context.Context, a domain.Actor, goalID domain.GoalID) ([]domain.GoalMember, error) {
	goal, err := readOne(ctx, r.db, a,
		"SELECT "+goalColumns+" FROM goals WHERE id = ?", []any{int64(goalID)}, scanGoal)
	if err != nil {
		return nil, err
	}
	scan := func(rows *sql.Rows) (domain.GoalMember, error) { return scanGoalMember(rows, goal) }
	return readAll(ctx, r.db, a,
		"SELECT "+membershipColumns+" "+membershipJoin+" WHERE gi.goal_id = ? ORDER BY i.title COLLATE NOCASE, i.id",
		[]any{int64(goalID)}, scan)
}

// goalForModify loads a goal for a mutation and re-checks modify permission, so
// a granted (read-only) or hidden goal never lets membership change.
func goalForModify(ctx context.Context, d *DB, a domain.Actor, id domain.GoalID) (domain.Goal, error) {
	goal, err := readOne(ctx, d, a,
		"SELECT "+goalColumns+" FROM goals WHERE id = ?", []any{int64(id)}, scanGoal)
	if err != nil {
		return domain.Goal{}, err
	}
	if err := domain.Authorize(a, domain.ActionModify, goal.Subject()); err != nil {
		return domain.Goal{}, err
	}
	return goal, nil
}

// scanGoalMember reads one membership row and resolves the per-goal tilt. A nil
// progress row leaves Tilt unset; the tilt owner always comes from the goal.
func scanGoalMember(rows *sql.Rows, goal domain.Goal) (domain.GoalMember, error) {
	var (
		it                    domain.Item
		kind                  string
		year                  sql.NullInt64
		externalID, coverPath sql.NullString
		owner                 sql.NullInt64
		created, updated      string
		pid, done             sql.NullInt64
		start, end            sql.NullString
	)
	if err := rows.Scan(&it.ID, &it.Title, &kind, &year, &externalID, &coverPath, &owner, &created, &updated,
		&pid, &done, &start, &end); err != nil {
		return domain.GoalMember{}, err
	}
	it.Kind = domain.Kind(kind)
	if year.Valid {
		y := int(year.Int64)
		it.Year = &y
	}
	it.ExternalID = externalID.String
	it.CoverPath = coverPath.String
	if owner.Valid {
		it.OwnerUserID = domain.UserID(owner.Int64)
	}
	it.CreatedAt, it.UpdatedAt = parseTime(created), parseTime(updated)

	tiltOwner := domain.TiltOwnerForGoal(goal)
	var tilt *domain.Progress
	if pid.Valid {
		p := domain.Progress{ID: pid.Int64, ItemID: it.ID, OwnerUserID: tiltOwner, Done: done.Int64 == 1}
		if start.Valid {
			d := parseDate(start.String)
			p.StartDate = &d
		}
		if end.Valid {
			d := parseDate(end.String)
			p.EndDate = &d
		}
		tilt = &p
	}
	return domain.NewGoalMember(it, tiltOwner, tilt, goal), nil
}

// parseDate reads a stored 'YYYY-MM-DD' date; malformed input degrades to the
// zero date rather than failing a read.
func parseDate(s string) domain.Date {
	t, _ := time.Parse(domain.DateLayout, s)
	return domain.Date{Time: t}
}
