package storage

import (
	"context"
	"database/sql"
	"time"

	"local-tracker/internal/domain"
)

// ItemRepo implements domain.ItemRepository.
type ItemRepo struct{ db *DB }

// itemColumns is the shared select list for every item read.
const itemColumns = "id, title, kind, year, external_id, cover_path, owner_user_id, created_at, updated_at"

// Create inserts one item and returns it with its assigned id.
func (r *ItemRepo) Create(ctx context.Context, a domain.Actor, it domain.Item) (domain.Item, error) {
	now := time.Now().UTC()
	it.CreatedAt, it.UpdatedAt = now, now
	res, err := r.db.write(ctx, a, it.Subject(), domain.ActionModify,
		"INSERT INTO items(title, kind, year, external_id, cover_path, owner_user_id, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)",
		it.Title, string(it.Kind), nullInt(it.Year), nullString(it.ExternalID), nullString(it.CoverPath),
		nullUser(it.OwnerUserID), formatTime(now), formatTime(now))
	if err != nil {
		return domain.Item{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return domain.Item{}, err
	}
	it.ID = domain.ItemID(id)
	return it, nil
}

// List returns every item the actor may view: shared items plus the actor's own.
func (r *ItemRepo) List(ctx context.Context, a domain.Actor) ([]domain.Item, error) {
	return readAll(ctx, r.db, a,
		"SELECT "+itemColumns+" FROM items ORDER BY title COLLATE NOCASE, id", nil, scanItem)
}

// Get returns one item or ErrForbidden when the actor may not view it. The row
// itself is fetched without a privacy predicate; readOne applies the single rule.
func (r *ItemRepo) Get(ctx context.Context, a domain.Actor, id domain.ItemID) (domain.Item, error) {
	return readOne(ctx, r.db, a,
		"SELECT "+itemColumns+" FROM items WHERE id = ?", []any{int64(id)}, scanItem)
}

// Update writes the editable fields of an existing item. The stored owner is
// reloaded and re-authorized so a forgotten check cannot leak a mutation.
func (r *ItemRepo) Update(ctx context.Context, a domain.Actor, it domain.Item) error {
	current, err := r.Get(ctx, a, it.ID)
	if err != nil {
		return err
	}
	_, err = r.db.write(ctx, a, current.Subject(), domain.ActionModify,
		"UPDATE items SET title=?, kind=?, year=?, external_id=?, owner_user_id=?, updated_at=? WHERE id=?",
		it.Title, string(it.Kind), nullInt(it.Year), nullString(it.ExternalID), nullUser(it.OwnerUserID),
		formatTime(time.Now().UTC()), int64(it.ID))
	return err
}

// SetCover stores only the server-generated filename on the item.
func (r *ItemRepo) SetCover(ctx context.Context, a domain.Actor, id domain.ItemID, coverPath string) error {
	current, err := r.Get(ctx, a, id)
	if err != nil {
		return err
	}
	_, err = r.db.write(ctx, a, current.Subject(), domain.ActionModify,
		"UPDATE items SET cover_path=?, updated_at=? WHERE id=?",
		nullString(coverPath), formatTime(time.Now().UTC()), int64(id))
	return err
}

// Delete removes one item. Memberships and progress cascade in the schema.
func (r *ItemRepo) Delete(ctx context.Context, a domain.Actor, id domain.ItemID) error {
	current, err := r.Get(ctx, a, id)
	if err != nil {
		return err
	}
	_, err = r.db.write(ctx, a, current.Subject(), domain.ActionModify,
		"DELETE FROM items WHERE id = ?", int64(id))
	return err
}

// scanItem reads one item row, mapping NULL columns to zero values.
func scanItem(rows *sql.Rows) (domain.Item, error) {
	var (
		it                    domain.Item
		kind                  string
		year                  sql.NullInt64
		externalID, coverPath sql.NullString
		owner                 sql.NullInt64
		created, updated      string
	)
	if err := rows.Scan(&it.ID, &it.Title, &kind, &year, &externalID, &coverPath, &owner, &created, &updated); err != nil {
		return domain.Item{}, err
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
	return it, nil
}

// nullInt maps an optional year to a SQL NULL when unset.
func nullInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

// nullString maps an empty string to a SQL NULL, keeping UNIQUE(external_id)
// usable because SQLite treats every NULL as distinct.
func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// nullUser maps the shared/zero owner to SQL NULL (G1).
func nullUser(id domain.UserID) any {
	if id == 0 {
		return nil
	}
	return int64(id)
}
