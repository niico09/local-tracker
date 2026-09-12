package domain

// SystemActorID is reserved for pre-authentication work: first-run setup,
// login and session lookup. Real user ids start at 1, so it cannot collide.
const SystemActorID UserID = -1

// Actor is the acting user. The id is unexported, so the zero value is the only
// forgeable one and a missing actor can never be mistaken for a real one.
type Actor struct{ id UserID }

// NewActor builds an actor for a known user id.
func NewActor(id UserID) Actor { return Actor{id: id} }

// SystemActor is the trusted process actor used before anyone has logged in.
func SystemActor() Actor { return Actor{id: SystemActorID} }

// ID returns the acting user id.
func (a Actor) ID() UserID { return a.id }

// Valid reports whether the actor carries a usable identity.
func (a Actor) Valid() bool { return a.id != 0 }

// Require fails with ErrNoActor when no acting user is present.
func (a Actor) Require() error {
	if !a.Valid() {
		return ErrNoActor
	}
	return nil
}
