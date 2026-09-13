package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"local-tracker/internal/domain"
)

// ReviewRepo implements domain.ReviewRepository. Ratings and the shared note
// hang off one catalog item, so every method authorizes through the item's
// subject: a personal item keeps its reviews private to its owner, a shared
// item exposes them to both partners.
type ReviewRepo struct{ db *DB }

// reviewItem loads the enclosing item and applies the single access rule for
// the requested action. Read paths authorize through readOne; writes re-check
// modify permission explicitly before any SQL runs.
func (r *ReviewRepo) reviewItem(ctx context.Context, a domain.Actor, itemID domain.ItemID, act domain.Action) (domain.Item, error) {
	item, err := readOne(ctx, r.db, a,
		"SELECT "+itemColumns+" FROM items WHERE id = ?", []any{int64(itemID)}, scanItem)
	if err != nil {
		return domain.Item{}, err
	}
	if act == domain.ActionModify {
		if err := domain.Authorize(a, act, item.Subject()); err != nil {
			return domain.Item{}, err
		}
	}
	return item, nil
}

// Ratings returns every stored rating of one item, ordered by user id.
func (r *ReviewRepo) Ratings(ctx context.Context, a domain.Actor, itemID domain.ItemID) ([]domain.Rating, error) {
	if _, err := r.reviewItem(ctx, a, itemID, domain.ActionView); err != nil {
		return nil, err
	}
	rows, err := r.db.read(ctx, a,
		"SELECT item_id, user_id, score, updated_at FROM item_ratings WHERE item_id = ? ORDER BY user_id", int64(itemID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []domain.Rating
	for rows.Next() {
		var (
			rating  domain.Rating
			updated string
		)
		if err := rows.Scan(&rating.ItemID, &rating.UserID, &rating.Score, &updated); err != nil {
			return nil, err
		}
		rating.UpdatedAt = parseTime(updated)
		out = append(out, rating)
	}
	return out, rows.Err()
}

// SetRating upserts one profile's score on a visible item. The subject guard
// makes any attempt to write another profile's rating fail closed even if the
// service ever passed the wrong user id.
func (r *ReviewRepo) SetRating(ctx context.Context, a domain.Actor, itemID domain.ItemID, userID domain.UserID, score int) (domain.Rating, error) {
	if _, err := r.reviewItem(ctx, a, itemID, domain.ActionModify); err != nil {
		return domain.Rating{}, err
	}
	now := time.Now().UTC()
	if _, err := r.db.write(ctx, a, domain.Subject{OwnerID: userID}, domain.ActionModify,
		"INSERT INTO item_ratings(item_id, user_id, score, updated_at) VALUES (?,?,?,?) "+
			"ON CONFLICT(item_id, user_id) DO UPDATE SET score=excluded.score, updated_at=excluded.updated_at",
		int64(itemID), int64(userID), score, formatTime(now)); err != nil {
		return domain.Rating{}, err
	}
	return domain.Rating{ItemID: itemID, UserID: userID, Score: score, UpdatedAt: now}, nil
}

// noteRow pairs a note with the enclosing item's ownership facts so the shared
// read helpers can authorize through the single access rule.
type noteRow struct {
	domain.Note
	subject domain.Subject
}

// Subject reports the enclosing item's ownership facts.
func (n noteRow) Subject() domain.Subject { return n.subject }

// Note returns the shared note; an absent note reads as an empty body.
func (r *ReviewRepo) Note(ctx context.Context, a domain.Actor, itemID domain.ItemID) (domain.Note, error) {
	item, err := r.reviewItem(ctx, a, itemID, domain.ActionView)
	if err != nil {
		return domain.Note{}, err
	}
	scan := func(rows *sql.Rows) (noteRow, error) { return scanNote(rows, item.Subject()) }
	row, err := readOne(ctx, r.db, a,
		"SELECT item_id, body, updated_at FROM item_notes WHERE item_id = ?", []any{int64(itemID)}, scan)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Note{ItemID: itemID}, nil
	}
	return row.Note, err
}

// SetNote upserts the shared note for a visible item.
func (r *ReviewRepo) SetNote(ctx context.Context, a domain.Actor, itemID domain.ItemID, body string) (domain.Note, error) {
	item, err := r.reviewItem(ctx, a, itemID, domain.ActionModify)
	if err != nil {
		return domain.Note{}, err
	}
	now := time.Now().UTC()
	if _, err := r.db.write(ctx, a, item.Subject(), domain.ActionModify,
		"INSERT INTO item_notes(item_id, body, updated_at) VALUES (?,?,?) "+
			"ON CONFLICT(item_id) DO UPDATE SET body=excluded.body, updated_at=excluded.updated_at",
		int64(itemID), body, formatTime(now)); err != nil {
		return domain.Note{}, err
	}
	return domain.Note{ItemID: itemID, Body: body, UpdatedAt: now}, nil
}

// DeleteNote removes the shared note; a missing row is not an error.
func (r *ReviewRepo) DeleteNote(ctx context.Context, a domain.Actor, itemID domain.ItemID) error {
	item, err := r.reviewItem(ctx, a, itemID, domain.ActionModify)
	if err != nil {
		return err
	}
	_, err = r.db.write(ctx, a, item.Subject(), domain.ActionModify,
		"DELETE FROM item_notes WHERE item_id = ?", int64(itemID))
	return err
}

// scanNote reads one note row, carrying the enclosing subject through.
func scanNote(rows *sql.Rows, subject domain.Subject) (noteRow, error) {
	var (
		n     domain.Note
		body  string
		added string
	)
	if err := rows.Scan(&n.ItemID, &body, &added); err != nil {
		return noteRow{}, err
	}
	n.Body = body
	n.UpdatedAt = parseTime(added)
	return noteRow{Note: n, subject: subject}, nil
}
