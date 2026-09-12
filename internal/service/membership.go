package service

import (
	"context"
	"fmt"

	"local-tracker/internal/domain"
)

// MembershipService implements the goal-membership use cases. Authorization is
// enforced by the repository (the enclosing goal is the protectable resource),
// so the service only validates input shape.
type MembershipService struct {
	members domain.MembershipRepository
}

// NewMembership wires the membership use cases.
func NewMembership(members domain.MembershipRepository) *MembershipService {
	return &MembershipService{members: members}
}

// ListMembers returns the items in a goal with their resolved tilt owner. A
// granted partner may list read-only; a hidden goal surfaces ErrForbidden.
func (s *MembershipService) ListMembers(ctx context.Context, a domain.Actor, goalID domain.GoalID) ([]domain.GoalMember, error) {
	if err := a.Require(); err != nil {
		return nil, err
	}
	if goalID <= 0 {
		return nil, fmt.Errorf("%w: goal id is required", domain.ErrValidation)
	}
	return s.members.ListMembers(ctx, a, goalID)
}

// Add links an item into a goal. A partner with only read visibility, or a
// hidden goal, is refused by the repository.
func (s *MembershipService) Add(ctx context.Context, a domain.Actor, goalID domain.GoalID, itemID domain.ItemID) error {
	if err := validateMembership(a, goalID, itemID); err != nil {
		return err
	}
	return s.members.Add(ctx, a, goalID, itemID)
}

// Remove deletes only the membership link, leaving the item and its progress
// untouched.
func (s *MembershipService) Remove(ctx context.Context, a domain.Actor, goalID domain.GoalID, itemID domain.ItemID) error {
	if err := validateMembership(a, goalID, itemID); err != nil {
		return err
	}
	return s.members.Remove(ctx, a, goalID, itemID)
}

// validateMembership enforces an acting user and positive ids for a single
// membership edge.
func validateMembership(a domain.Actor, goalID domain.GoalID, itemID domain.ItemID) error {
	if err := a.Require(); err != nil {
		return err
	}
	if goalID <= 0 {
		return fmt.Errorf("%w: goal id is required", domain.ErrValidation)
	}
	if itemID <= 0 {
		return fmt.Errorf("%w: item id is required", domain.ErrValidation)
	}
	return nil
}
