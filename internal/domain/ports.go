package domain

import (
	"context"
	"time"
)

// UserRepository persists the two profiles. Every method requires an Actor so
// no query can run unscoped.
type UserRepository interface {
	Count(ctx context.Context, a Actor) (int, error)
	Setup(ctx context.Context, a Actor, first, second User) error
	FindByName(ctx context.Context, a Actor, name string) (User, error)
	FindByID(ctx context.Context, a Actor, id UserID) (User, error)
}

// Session maps one login to a user. Only TokenHash is stored.
type Session struct {
	TokenHash []byte
	UserID    UserID
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionRepository persists login sessions. Every method requires an Actor.
type SessionRepository interface {
	Create(ctx context.Context, a Actor, s Session) error
	FindByTokenHash(ctx context.Context, a Actor, tokenHash []byte) (Session, error)
	Delete(ctx context.Context, a Actor, tokenHash []byte) error
	DeleteExpired(ctx context.Context, a Actor, now time.Time) (int, error)
}
