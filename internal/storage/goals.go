package storage

import (
	"context"
	"database/sql"
	"time"

	"local-tracker/internal/domain"
)

// GoalRepo implements domain.GoalRepository.
type GoalRepo struct{ db *DB }

// goalColumns is the shared select list for every goal read.
const goalColumns = "id, title, owner_user_id, target, visibility, external_key, created_at, updated_at"

// Create inserts one goal and returns it with its assigned id. A couple goal
// carries owner 0 (NULL) and is therefore writable by either partner.
func (r *GoalRepo) Create(ctx context.Context, a domain.Actor, g domain.Goal) (domain.Goal, error) {
	now := time.Now().UTC()
	g.CreatedAt, g.UpdatedAt = now, now
	res, err := r.db.write(ctx, a, g.Subject(), domain.ActionModify,
		"INSERT INTO goals(title, owner_user_id, target, visibility, external_key, created_at, updated_at) VALUES (?,?,?,?,?,?,?)",
		g.Title, nullUser(g.OwnerUserID), nullInt(g.Target), string(g.Visibility),
		nullString(g.ExternalKey), formatTime(now), formatTime(now))
	if err != nil {
		return domain.Goal{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.Goal{}, err
	}
	g.ID = domain.GoalID(id)
	return g, nil
}

// List returns every goal the actor may view: couple goals plus the actor's own
// and any personal goal the owner granted read-only visibility.
func (r *GoalRepo) List(ctx context.Context, a domain.Actor) ([]domain.Goal, error) {
	return readAll(ctx, r.db, a,
		"SELECT "+goalColumns+" FROM goals ORDER BY title COLLATE NOCASE, id", nil, scanGoal)
}

// Get returns one goal or ErrForbidden when the actor may not view it. The row
// is fetched without a privacy predicate; readOne applies the single rule.
func (r *GoalRepo) Get(ctx context.Context, a domain.Actor, id domain.GoalID) (domain.Goal, error) {
	return readOne(ctx, r.db, a,
		"SELECT "+goalColumns+" FROM goals WHERE id = ?", []any{int64(id)}, scanGoal)
}

// Update writes the editable fields of an existing goal. Ownership and the
// external key are preserved, and the stored subject is re-authorized so a
// granted partner (view-only) can never mutate the goal.
func (r *GoalRepo) Update(ctx context.Context, a domain.Actor, g domain.Goal) error {
	current, err := r.Get(ctx, a, g.ID)
	if err != nil {
		return err
	}
	_, err = r.db.write(ctx, a, current.Subject(), domain.ActionModify,
		"UPDATE goals SET title=?, target=?, visibility=?, updated_at=? WHERE id=?",
		g.Title, nullInt(g.Target), string(g.Visibility), formatTime(time.Now().UTC()), int64(g.ID))
	return err
}

// SetVisibility grants or revokes partner visibility. Only the owner (or either
// partner on a couple goal) may change it.
func (r *GoalRepo) SetVisibility(ctx context.Context, a domain.Actor, id domain.GoalID, v domain.Visibility) error {
	current, err := r.Get(ctx, a, id)
	if err != nil {
		return err
	}
	_, err = r.db.write(ctx, a, current.Subject(), domain.ActionModify,
		"UPDATE goals SET visibility=?, updated_at=? WHERE id=?",
		string(v), formatTime(time.Now().UTC()), int64(id))
	return err
}

// Delete removes one goal. Membership and any dependent rows cascade in the
// schema.
func (r *GoalRepo) Delete(ctx context.Context, a domain.Actor, id domain.GoalID) error {
	current, err := r.Get(ctx, a, id)
	if err != nil {
		return err
	}
	_, err = r.db.write(ctx, a, current.Subject(), domain.ActionModify,
		"DELETE FROM goals WHERE id = ?", int64(id))
	return err
}

// scanGoal reads one goal row, mapping NULL columns to zero values.
func scanGoal(rows *sql.Rows) (domain.Goal, error) {
	var (
		g                  domain.Goal
		owner, target      sql.NullInt64
		visibility         string
		externalKey        sql.NullString
		created, updatedAt string
	)
	if err := rows.Scan(&g.ID, &g.Title, &owner, &target, &visibility, &externalKey, &created, &updatedAt); err != nil {
		return domain.Goal{}, err
	}
	if owner.Valid {
		g.OwnerUserID = domain.UserID(owner.Int64)
	}
	if target.Valid {
		t := int(target.Int64)
		g.Target = &t
	}
	g.Visibility = domain.Visibility(visibility)
	g.ExternalKey = externalKey.String
	g.CreatedAt, g.UpdatedAt = parseTime(created), parseTime(updatedAt)
	return g, nil
}
