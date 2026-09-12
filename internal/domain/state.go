package domain

import "time"

// State is the derived progress state. It is never stored: `done` plus the
// optional dates determine it at read time (G2/I7).
type State string

const (
	// StatePending means no progress has started.
	StatePending State = "pending"
	// StateInProgress means a start date exists and done is false.
	StateInProgress State = "in_progress"
	// StateCompleted means done is true.
	StateCompleted State = "completed"
)

// DateLayout is the storage and form layout for calendar dates.
const DateLayout = "2006-01-02"

// Date is a calendar date with no time or zone.
type Date struct{ time.Time }

// NewDate builds a Date at UTC midnight.
func NewDate(year int, month time.Month, day int) Date {
	return Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// String renders the storage layout.
func (d Date) String() string { return d.Format(DateLayout) }

// Before reports whether d falls strictly before other.
func (d Date) Before(other Date) bool { return d.Time.Before(other.Time) }

// Progress is one owner's tilt on one item. OwnerUserID 0 is the shared couple
// tilt. The progress repository and HTTP layers arrive in slice S5; the entity
// lives here because DeriveState and ValidateDates are pure domain rules.
type Progress struct {
	ID          int64
	ItemID      ItemID
	OwnerUserID UserID
	Done        bool
	StartDate   *Date
	EndDate     *Date
}

// DeriveState implements the done-authoritative rule (G2/I7): completion comes
// from `done` alone; a start date without done is in-progress; otherwise pending.
func DeriveState(p Progress) State {
	if p.Done {
		return StateCompleted
	}
	if p.StartDate != nil {
		return StateInProgress
	}
	return StatePending
}

// ValidateDates enforces end >= start when both dates are present (I6).
func ValidateDates(start, end *Date) error {
	if start != nil && end != nil && end.Before(*start) {
		return ErrValidation
	}
	return nil
}
