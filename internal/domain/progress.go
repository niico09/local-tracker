package domain

// The Progress and Date value types live in state.go because DeriveState and
// ValidateDates are pure domain rules (slice S2). This file adds the
// authorization face and repository-facing behavior of a progress row.

// Subject exposes the tilt owner's ownership facts to the single access rule.
// OwnerUserID 0 is the shared couple tilt, which both partners may read and
// write; a personal tilt is visible and writable only by its owner. A progress
// row never carries a read-only partner grant of its own: read-only visibility
// of a personal goal is enforced on the enclosing goal, not on the tilt.
func (p Progress) Subject() Subject { return Subject{OwnerID: p.OwnerUserID} }
