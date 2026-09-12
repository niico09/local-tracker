package service

import (
	"context"
	"fmt"

	"local-tracker/internal/domain"
)

// ProgressService implements the progress use cases on top of the tilt port. The
// repository resolves the tilt owner server-side (Rule 1 inside a goal, the item
// owner standalone) and enforces authorization, so the service validates shape
// and owns the toggle/dates semantics.
type ProgressService struct {
	progress domain.ProgressRepository
}

// NewProgress wires the progress use cases.
func NewProgress(progress domain.ProgressRepository) *ProgressService {
	return &ProgressService{progress: progress}
}

// Tilt returns the resolved tilt for an item; goalID 0 selects the standalone
// tilt. An absent row reads as pending rather than an error.
func (s *ProgressService) Tilt(ctx context.Context, a domain.Actor, goalID domain.GoalID, itemID domain.ItemID) (domain.Progress, error) {
	if err := validateTiltArgs(a, goalID, itemID); err != nil {
		return domain.Progress{}, err
	}
	return s.progress.Tilt(ctx, a, goalID, itemID)
}

// Toggle flips done on the resolved tilt and returns the stored row. Dates are
// preserved: completion comes from done alone (G2/I7).
func (s *ProgressService) Toggle(ctx context.Context, a domain.Actor, goalID domain.GoalID, itemID domain.ItemID) (domain.Progress, error) {
	current, err := s.Tilt(ctx, a, goalID, itemID)
	if err != nil {
		return domain.Progress{}, err
	}
	current.Done = !current.Done
	return s.progress.SetTilt(ctx, a, goalID, current)
}

// SetDates writes the optional dates plus the done flag, enforcing end >= start
// (I6). No dates with done false reads pending; a start date reads in-progress;
// done reads completed (G2/Rule 5).
func (s *ProgressService) SetDates(ctx context.Context, a domain.Actor, goalID domain.GoalID, itemID domain.ItemID, done bool, start, end *domain.Date) (domain.Progress, error) {
	if err := validateTiltArgs(a, goalID, itemID); err != nil {
		return domain.Progress{}, err
	}
	if err := domain.ValidateDates(start, end); err != nil {
		return domain.Progress{}, err
	}
	return s.progress.SetTilt(ctx, a, goalID, domain.Progress{
		ItemID:    itemID,
		Done:      done,
		StartDate: start,
		EndDate:   end,
	})
}

// GoalRollup returns the display numbers for a goal: done over the target when
// set, otherwise over the current member count (G3/I8).
func (s *ProgressService) GoalRollup(ctx context.Context, a domain.Actor, goalID domain.GoalID) (done, total int, err error) {
	if err := a.Require(); err != nil {
		return 0, 0, err
	}
	if goalID <= 0 {
		return 0, 0, fmt.Errorf("%w: goal id is required", domain.ErrValidation)
	}
	return s.progress.GoalRollup(ctx, a, goalID)
}

// validateTiltArgs enforces an acting user, a positive item, and an optional
// goal (0 is the standalone tilt).
func validateTiltArgs(a domain.Actor, goalID domain.GoalID, itemID domain.ItemID) error {
	if err := a.Require(); err != nil {
		return err
	}
	if goalID < 0 {
		return fmt.Errorf("%w: invalid goal", domain.ErrValidation)
	}
	if itemID <= 0 {
		return fmt.Errorf("%w: item id is required", domain.ErrValidation)
	}
	return nil
}
