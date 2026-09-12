package domain

// Action is the kind of operation being authorized.
type Action uint8

const (
	// ActionView reads a resource.
	ActionView Action = iota
	// ActionModify changes a resource.
	ActionModify
)

// Subject is the ownership facts of any protectable row.
type Subject struct {
	OwnerID UserID // 0 = couple/shared
	Granted bool   // personal resource shared read-only with the partner
}

// Authorize is the ONLY authorization decision in the system. Every read and
// every mutation resolves through it; a missing actor always fails.
func Authorize(a Actor, act Action, s Subject) error {
	if err := a.Require(); err != nil {
		return err
	}
	if s.OwnerID == 0 { // couple resource: both partners
		return nil
	}
	if s.OwnerID == a.ID() { // owner: full access
		return nil
	}
	if act == ActionView && s.Granted { // granted: read-only
		return nil
	}
	return ErrForbidden // hidden by default
}
