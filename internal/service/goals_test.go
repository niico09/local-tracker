package service

import (
	"context"
	"errors"
	"testing"

	"local-tracker/internal/domain"
)

// stubGoalRepo records the goal the service hands to Create so validation can be
// asserted without a database.
type stubGoalRepo struct {
	created domain.Goal
}

func (s *stubGoalRepo) List(context.Context, domain.Actor) ([]domain.Goal, error) { return nil, nil }
func (s *stubGoalRepo) Get(context.Context, domain.Actor, domain.GoalID) (domain.Goal, error) {
	return domain.Goal{}, domain.ErrNotFound
}
func (s *stubGoalRepo) Create(_ context.Context, _ domain.Actor, g domain.Goal) (domain.Goal, error) {
	s.created = g
	return g, nil
}
func (s *stubGoalRepo) Update(context.Context, domain.Actor, domain.Goal) error { return nil }
func (s *stubGoalRepo) SetVisibility(context.Context, domain.Actor, domain.GoalID, domain.Visibility) error {
	return nil
}
func (s *stubGoalRepo) Delete(context.Context, domain.Actor, domain.GoalID) error { return nil }

func TestGoalValidation(t *testing.T) {
	actor := domain.NewActor(1)
	zero, negative, valid := 0, -3, 10

	cases := []struct {
		name    string
		in      GoalInput
		wantErr bool
	}{
		{"blank title", GoalInput{Title: "   ", OwnerUserID: 1}, true},
		{"zero target", GoalInput{Title: "x", OwnerUserID: 1, Target: &zero}, true},
		{"negative target", GoalInput{Title: "x", OwnerUserID: 1, Target: &negative}, true},
		{"unknown visibility", GoalInput{Title: "x", OwnerUserID: 1, Visibility: domain.Visibility("public")}, true},
		{"foreign owner", GoalInput{Title: "x", OwnerUserID: 2}, true},
		{"couple goal", GoalInput{Title: "x", OwnerUserID: 0}, false},
		{"personal goal", GoalInput{Title: "x", OwnerUserID: 1, Target: &valid}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewGoals(&stubGoalRepo{})
			_, err := svc.Create(context.Background(), actor, tc.in)
			if tc.wantErr {
				if !errors.Is(err, domain.ErrValidation) {
					t.Fatalf("err = %v, want ErrValidation", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
		})
	}
}

// TestGoalDefaults proves trimming and the private default visibility.
func TestGoalDefaults(t *testing.T) {
	repo := &stubGoalRepo{}
	svc := NewGoals(repo)
	created, err := svc.Create(context.Background(), domain.NewActor(1), GoalInput{Title: "  Reading  ", OwnerUserID: 0})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Title != "Reading" {
		t.Fatalf("title = %q, want trimmed Reading", created.Title)
	}
	if created.Visibility != domain.VisibilityPrivate {
		t.Fatalf("visibility = %q, want private default", created.Visibility)
	}
	if repo.created.OwnerUserID != 0 {
		t.Fatalf("couple owner = %d, want 0", repo.created.OwnerUserID)
	}
}
