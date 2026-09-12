package service

import (
	"context"
	"errors"
	"testing"

	"local-tracker/internal/domain"
)

// stubMembershipRepo records calls so the service's validation and delegation
// can be asserted without a database.
type stubMembershipRepo struct {
	added   bool
	removed bool
}

func (s *stubMembershipRepo) Add(context.Context, domain.Actor, domain.GoalID, domain.ItemID) error {
	s.added = true
	return nil
}

func (s *stubMembershipRepo) Remove(context.Context, domain.Actor, domain.GoalID, domain.ItemID) error {
	s.removed = true
	return nil
}

func (s *stubMembershipRepo) ListMembers(context.Context, domain.Actor, domain.GoalID) ([]domain.GoalMember, error) {
	return nil, nil
}

func TestMembershipValidation(t *testing.T) {
	actor := domain.NewActor(1)
	svc := NewMembership(&stubMembershipRepo{})

	cases := []struct {
		name    string
		run     func() error
		wantErr error
	}{
		{"zero goal id", func() error { return svc.Add(context.Background(), actor, 0, 1) }, domain.ErrValidation},
		{"zero item id", func() error { return svc.Add(context.Background(), actor, 1, 0) }, domain.ErrValidation},
		{"negative item id", func() error { return svc.Remove(context.Background(), actor, 1, -2) }, domain.ErrValidation},
		{"zero goal on list", func() error {
			_, err := svc.ListMembers(context.Background(), actor, 0)
			return err
		}, domain.ErrValidation},
		{"no actor on add", func() error { return svc.Add(context.Background(), domain.Actor{}, 1, 1) }, domain.ErrNoActor},
		{"no actor on list", func() error {
			_, err := svc.ListMembers(context.Background(), domain.Actor{}, 1)
			return err
		}, domain.ErrNoActor},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestMembershipDelegatesToRepository(t *testing.T) {
	repo := &stubMembershipRepo{}
	svc := NewMembership(repo)
	ctx := context.Background()
	actor := domain.NewActor(1)

	if err := svc.Add(ctx, actor, 1, 2); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !repo.added {
		t.Fatal("Add did not reach the repository")
	}
	if err := svc.Remove(ctx, actor, 1, 2); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !repo.removed {
		t.Fatal("Remove did not reach the repository")
	}
}
