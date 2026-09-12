package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"local-tracker/internal/domain"
)

// stubProgressRepo records the tilt the service hands to SetTilt so toggle and
// dates semantics can be asserted without a database.
type stubProgressRepo struct {
	tilt      domain.Progress
	set       domain.Progress
	setCalled bool
}

func (s *stubProgressRepo) Tilt(context.Context, domain.Actor, domain.GoalID, domain.ItemID) (domain.Progress, error) {
	return s.tilt, nil
}

func (s *stubProgressRepo) SetTilt(_ context.Context, _ domain.Actor, _ domain.GoalID, p domain.Progress) (domain.Progress, error) {
	s.set = p
	s.setCalled = true
	return p, nil
}

func (s *stubProgressRepo) GoalRollup(context.Context, domain.Actor, domain.GoalID) (int, int, error) {
	return 0, 0, nil
}

func datePtr(y int, m time.Month, d int) *domain.Date {
	v := domain.NewDate(y, m, d)
	return &v
}

// TestProgressValidation covers the service-level shape checks.
func TestProgressValidation(t *testing.T) {
	actor := domain.NewActor(1)
	svc := NewProgress(&stubProgressRepo{})

	cases := []struct {
		name    string
		run     func() error
		wantErr error
	}{
		{"no actor on tilt", func() error {
			_, err := svc.Tilt(context.Background(), domain.Actor{}, 0, 1)
			return err
		}, domain.ErrNoActor},
		{"zero item", func() error {
			_, err := svc.Tilt(context.Background(), actor, 0, 0)
			return err
		}, domain.ErrValidation},
		{"negative goal", func() error {
			_, err := svc.Tilt(context.Background(), actor, -1, 1)
			return err
		}, domain.ErrValidation},
		{"end before start", func() error {
			_, err := svc.SetDates(context.Background(), actor, 0, 1, false,
				datePtr(2024, time.February, 1), datePtr(2024, time.January, 1))
			return err
		}, domain.ErrValidation},
		{"no actor on rollup", func() error {
			_, _, err := svc.GoalRollup(context.Background(), domain.Actor{}, 1)
			return err
		}, domain.ErrNoActor},
		{"zero goal on rollup", func() error {
			_, _, err := svc.GoalRollup(context.Background(), actor, 0)
			return err
		}, domain.ErrValidation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// TestProgressToggleFlips proves toggle reads the current tilt and writes the
// opposite done flag, preserving the dates.
func TestProgressToggleFlips(t *testing.T) {
	start := datePtr(2024, time.January, 2)
	repo := &stubProgressRepo{tilt: domain.Progress{ItemID: 7, Done: false, StartDate: start}}
	svc := NewProgress(repo)

	stored, err := svc.Toggle(context.Background(), domain.NewActor(1), 0, 7)
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if !repo.setCalled || !stored.Done || repo.set.StartDate == nil {
		t.Fatalf("toggle wrote %+v (called=%v), want done true and the start date kept", repo.set, repo.setCalled)
	}
}

// TestProgressSetDatesDelegates proves the validated dates and done flag reach
// the repository unchanged.
func TestProgressSetDatesDelegates(t *testing.T) {
	repo := &stubProgressRepo{}
	svc := NewProgress(repo)
	start := datePtr(2024, time.March, 1)
	end := datePtr(2024, time.March, 31)

	if _, err := svc.SetDates(context.Background(), domain.NewActor(1), 0, 5, true, start, end); err != nil {
		t.Fatalf("SetDates: %v", err)
	}
	if !repo.setCalled || !repo.set.Done || repo.set.StartDate == nil || repo.set.EndDate == nil {
		t.Fatalf("SetDates wrote %+v, want done and both dates", repo.set)
	}
}
