package service

import (
	"context"
	"fmt"
	"strings"

	"local-tracker/internal/domain"
)

// GoalInput is the validated user input for creating or updating a goal.
// OwnerUserID 0 means a couple goal; a non-zero owner must be the actor.
type GoalInput struct {
	Title       string
	OwnerUserID domain.UserID
	Target      *int
	Visibility  domain.Visibility
}

// GoalService implements the goals use cases on top of the goal port.
type GoalService struct {
	goals domain.GoalRepository
}

// NewGoals wires the goals use cases.
func NewGoals(goals domain.GoalRepository) *GoalService {
	return &GoalService{goals: goals}
}

// List returns the goals visible to the actor.
func (s *GoalService) List(ctx context.Context, a domain.Actor) ([]domain.Goal, error) {
	return s.goals.List(ctx, a)
}

// Get returns one goal or ErrForbidden when it is hidden from the actor.
func (s *GoalService) Get(ctx context.Context, a domain.Actor, id domain.GoalID) (domain.Goal, error) {
	return s.goals.Get(ctx, a, id)
}

// Create validates input and inserts one goal.
func (s *GoalService) Create(ctx context.Context, a domain.Actor, in GoalInput) (domain.Goal, error) {
	goal, err := validateGoal(a, in)
	if err != nil {
		return domain.Goal{}, err
	}
	return s.goals.Create(ctx, a, goal)
}

// Update validates input and applies it to an existing goal. Ownership and the
// external key are preserved: editing never reassigns a goal.
func (s *GoalService) Update(ctx context.Context, a domain.Actor, id domain.GoalID, in GoalInput) (domain.Goal, error) {
	current, err := s.goals.Get(ctx, a, id)
	if err != nil {
		return domain.Goal{}, err
	}
	goal, err := validateGoal(a, in)
	if err != nil {
		return domain.Goal{}, err
	}
	goal.ID = current.ID
	goal.OwnerUserID = current.OwnerUserID
	goal.ExternalKey = current.ExternalKey
	if err := s.goals.Update(ctx, a, goal); err != nil {
		return domain.Goal{}, err
	}
	return goal, nil
}

// SetVisibility grants or revokes partner visibility on a personal goal. The
// repository re-authorizes the stored subject, so a granted partner cannot
// change it.
func (s *GoalService) SetVisibility(ctx context.Context, a domain.Actor, id domain.GoalID, v domain.Visibility) error {
	if err := a.Require(); err != nil {
		return err
	}
	if !v.Valid() {
		return fmt.Errorf("%w: unknown visibility %q", domain.ErrValidation, v)
	}
	return s.goals.SetVisibility(ctx, a, id, v)
}

// Delete removes a goal the actor owns, or a couple goal.
func (s *GoalService) Delete(ctx context.Context, a domain.Actor, id domain.GoalID) error {
	return s.goals.Delete(ctx, a, id)
}

// validateGoal enforces the goal invariants: a required title, an optional
// target that must be positive, a known visibility (private by default), and an
// owner that is either the couple (0) or the acting user.
func validateGoal(a domain.Actor, in GoalInput) (domain.Goal, error) {
	if err := a.Require(); err != nil {
		return domain.Goal{}, err
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return domain.Goal{}, fmt.Errorf("%w: title is required", domain.ErrValidation)
	}
	if in.Target != nil && *in.Target <= 0 {
		return domain.Goal{}, fmt.Errorf("%w: target must be greater than zero", domain.ErrValidation)
	}
	visibility := in.Visibility
	if visibility == "" {
		visibility = domain.VisibilityPrivate
	}
	if !visibility.Valid() {
		return domain.Goal{}, fmt.Errorf("%w: unknown visibility %q", domain.ErrValidation, visibility)
	}
	if in.OwnerUserID != 0 && in.OwnerUserID != a.ID() {
		return domain.Goal{}, fmt.Errorf("%w: cannot own a goal for another profile", domain.ErrValidation)
	}
	return domain.Goal{
		Title:       title,
		OwnerUserID: in.OwnerUserID,
		Target:      in.Target,
		Visibility:  visibility,
	}, nil
}
