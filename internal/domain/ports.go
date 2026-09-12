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

// ItemRepository persists the global catalog. Every method requires an Actor so
// no query can run unscoped and every returned row is authorized centrally.
type ItemRepository interface {
	List(ctx context.Context, a Actor) ([]Item, error)
	Get(ctx context.Context, a Actor, id ItemID) (Item, error)
	Create(ctx context.Context, a Actor, it Item) (Item, error)
	Update(ctx context.Context, a Actor, it Item) error
	SetCover(ctx context.Context, a Actor, id ItemID, coverPath string) error
	Delete(ctx context.Context, a Actor, id ItemID) error
}

// MembershipRepository persists goal_items links and resolves the per-goal tilt
// (Rule 1). Every method requires an Actor; authorization is applied to the
// enclosing goal, so a partner with read-only visibility can list members but
// may never add or remove one.
type MembershipRepository interface {
	Add(ctx context.Context, a Actor, goalID GoalID, itemID ItemID) error
	Remove(ctx context.Context, a Actor, goalID GoalID, itemID ItemID) error
	ListMembers(ctx context.Context, a Actor, goalID GoalID) ([]GoalMember, error)
}

// GoalRepository persists couple and personal goals. Every method requires an
// Actor so no query can run unscoped and every returned row is authorized
// centrally; a personal goal with VisibilityShared is readable by the partner
// but only the owner may modify it.
type GoalRepository interface {
	List(ctx context.Context, a Actor) ([]Goal, error)
	Get(ctx context.Context, a Actor, id GoalID) (Goal, error)
	Create(ctx context.Context, a Actor, g Goal) (Goal, error)
	Update(ctx context.Context, a Actor, g Goal) error
	SetVisibility(ctx context.Context, a Actor, id GoalID, v Visibility) error
	Delete(ctx context.Context, a Actor, id GoalID) error
}
