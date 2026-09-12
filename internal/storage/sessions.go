package storage

import (
	"context"
	"time"

	"local-tracker/internal/domain"
)

// SessionRepo implements domain.SessionRepository.
type SessionRepo struct{ db *DB }

// Create stores a session row keyed by the token hash.
func (r *SessionRepo) Create(ctx context.Context, a domain.Actor, s domain.Session) error {
	_, err := r.db.write(ctx, a, domain.Subject{}, domain.ActionModify,
		"INSERT INTO sessions(token_hash, user_id, created_at, expires_at) VALUES (?,?,?,?)",
		s.TokenHash, s.UserID, formatTime(s.CreatedAt), formatTime(s.ExpiresAt))
	return err
}

// FindByTokenHash loads the session for a hash, or ErrNotFound.
func (r *SessionRepo) FindByTokenHash(ctx context.Context, a domain.Actor, tokenHash []byte) (domain.Session, error) {
	rows, err := r.db.read(ctx, a,
		"SELECT token_hash, user_id, created_at, expires_at FROM sessions WHERE token_hash = ?", tokenHash)
	if err != nil {
		return domain.Session{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return domain.Session{}, err
		}
		return domain.Session{}, domain.ErrNotFound
	}
	var s domain.Session
	var created, expires string
	if err := rows.Scan(&s.TokenHash, &s.UserID, &created, &expires); err != nil {
		return domain.Session{}, err
	}
	s.CreatedAt, s.ExpiresAt = parseTime(created), parseTime(expires)
	return s, rows.Err()
}

// Delete revokes one session by its token hash.
func (r *SessionRepo) Delete(ctx context.Context, a domain.Actor, tokenHash []byte) error {
	_, err := r.db.write(ctx, a, domain.Subject{}, domain.ActionModify,
		"DELETE FROM sessions WHERE token_hash = ?", tokenHash)
	return err
}

// DeleteExpired removes sessions whose expiry has passed.
func (r *SessionRepo) DeleteExpired(ctx context.Context, a domain.Actor, now time.Time) (int, error) {
	res, err := r.db.write(ctx, a, domain.Subject{}, domain.ActionModify,
		"DELETE FROM sessions WHERE expires_at <= ?", formatTime(now))
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}
