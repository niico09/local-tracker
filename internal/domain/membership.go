package domain

import "time"

// GoalItem is one membership link between a goal and a catalog item. Removing a
// link never touches the item nor its progress rows; only the link disappears.
type GoalItem struct {
	GoalID  GoalID
	ItemID  ItemID
	AddedAt time.Time
}

// TiltOwnerForGoal resolves which progress owner a tilt uses inside a goal
// (Rule 1 / I4): the goal owner rules. A couple goal (owner 0) therefore reads
// and writes the shared tilt, while a personal goal uses its owner's tilt.
func TiltOwnerForGoal(g Goal) UserID { return g.OwnerUserID }

// TiltOwnerForItem resolves the standalone tilt owner outside goals (G5/I5):
// the item's own owner, 0 meaning shared. It is only meaningful on catalog
// paths; inside a goal TiltOwnerForGoal always wins.
func TiltOwnerForItem(i Item) UserID { return i.OwnerUserID }

// GoalMember is a catalog item as seen inside one goal. It carries the tilt
// owner Rule 1 resolves for that goal and the resolved progress row when one
// exists. Authorization is bound to the enclosing goal, not the item: a partner
// granted view of the goal reads its members even when the item itself is
// personal to the goal's owner.
type GoalMember struct {
	Item
	TiltOwner UserID    // Rule 1 tilt owner; 0 = shared couple tilt
	Tilt      *Progress // resolved progress row for that tilt, nil when absent
	subject   Subject
}

// NewGoalMember binds an item to its enclosing goal, resolving the Rule 1 tilt
// owner and adopting the goal's ownership facts as the authorizable subject.
func NewGoalMember(item Item, tiltOwner UserID, tilt *Progress, goal Goal) GoalMember {
	return GoalMember{Item: item, TiltOwner: tiltOwner, Tilt: tilt, subject: goal.Subject()}
}

// Subject reports the enclosing goal's ownership facts: the goal is the
// protectable resource for every membership read.
func (m GoalMember) Subject() Subject { return m.subject }
