// Package domain holds the pure model and invariants. It imports no internal
// package: no HTTP, no SQL, no filesystem.
package domain

import "errors"

var (
	// ErrNotFound means the requested row does not exist.
	ErrNotFound = errors.New("not found")
	// ErrForbidden means the actor may not see or change the resource.
	ErrForbidden = errors.New("forbidden")
	// ErrValidation means the submitted input failed a domain rule.
	ErrValidation = errors.New("validation failed")
	// ErrNoActor means a call ran without an acting user.
	ErrNoActor = errors.New("no acting user")
)
