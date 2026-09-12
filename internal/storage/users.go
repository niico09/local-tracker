package storage

import (
	"context"

	"local-tracker/internal/domain"
)

// UserRepo implements domain.UserRepository.
type UserRepo struct{ db *DB }

// Count returns how many profiles exist.
func (r *UserRepo) Count(ctx context.Context, a domain.Actor) (int, error) {
	return r.db.count(ctx, a, "SELECT COUNT(*) FROM users")
}

// Setup creates both profiles inside one transaction, re-checking inside the
// transaction that no user exists yet. It is the only user-creation path.
func (r *UserRepo) Setup(ctx context.Context, a domain.Actor, first, second domain.User) error {
	if err := a.Require(); err != nil {
		return err
	}
	tx, err := r.db.raw.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var n int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&n); err != nil {
		return err
	}
	if n != 0 {
		return domain.ErrForbidden
	}
	for _, u := range []domain.User{first, second} {
		if _, err := writeTx(ctx, tx, a, domain.Subject{}, domain.ActionModify,
			"INSERT INTO users(name, pin_hash, pin_salt, pin_iter, created_at) VALUES (?,?,?,?,?)",
			u.Name, u.PinHash, u.PinSalt, u.PinIter, formatTime(u.CreatedAt)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// FindByName loads a profile by exact name.
func (r *UserRepo) FindByName(ctx context.Context, a domain.Actor, name string) (domain.User, error) {
	return r.find(ctx, a, "SELECT id, name, pin_hash, pin_salt, pin_iter, created_at FROM users WHERE name = ?", name)
}

// FindByID loads a profile by id.
func (r *UserRepo) FindByID(ctx context.Context, a domain.Actor, id domain.UserID) (domain.User, error) {
	return r.find(ctx, a, "SELECT id, name, pin_hash, pin_salt, pin_iter, created_at FROM users WHERE id = ?", id)
}

func (r *UserRepo) find(ctx context.Context, a domain.Actor, q string, arg any) (domain.User, error) {
	rows, err := r.db.read(ctx, a, q, arg)
	if err != nil {
		return domain.User{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return domain.User{}, err
		}
		return domain.User{}, domain.ErrNotFound
	}
	var u domain.User
	var created string
	if err := rows.Scan(&u.ID, &u.Name, &u.PinHash, &u.PinSalt, &u.PinIter, &created); err != nil {
		return domain.User{}, err
	}
	u.CreatedAt = parseTime(created)
	return u, rows.Err()
}
