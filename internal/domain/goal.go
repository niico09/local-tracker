package domain

import "time"

// GoalID identifies a goal.
type GoalID int64

// Visibility controls whether a personal goal is readable by the partner.
type Visibility string

const (
	// VisibilityPrivate hides a personal goal from the partner (default).
	VisibilityPrivate Visibility = "private"
	// VisibilityShared grants the partner read-only access to a personal goal.
	VisibilityShared Visibility = "shared"
)

// Visibilities lists the allowed visibility values in display order.
func Visibilities() []Visibility { return []Visibility{VisibilityPrivate, VisibilityShared} }

// Valid reports whether v is one of the two allowed visibility values (I10).
func (v Visibility) Valid() bool {
	switch v {
	case VisibilityPrivate, VisibilityShared:
		return true
	}
	return false
}

// Goal is a couple goal (OwnerUserID 0) or a personal goal owned by one
// profile. Target is optional and, when set, is always greater than zero.
type Goal struct {
	ID          GoalID
	Title       string
	OwnerUserID UserID // 0 = couple goal shared by both partners
	Target      *int   // nil = no target; otherwise > 0
	Visibility  Visibility
	ExternalKey string // 'top100' seed idempotency; empty for user goals
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Subject exposes the goal's ownership facts to the single access rule. A
// personal goal with VisibilityShared grants the partner read-only access;
// ActionModify still requires ownership.
func (g Goal) Subject() Subject {
	return Subject{OwnerID: g.OwnerUserID, Granted: g.Visibility == VisibilityShared}
}
