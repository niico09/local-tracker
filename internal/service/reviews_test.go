package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"local-tracker/internal/domain"
)

// stubReviewRepo records calls so service validation and defaults can be
// asserted without a database.
type stubReviewRepo struct {
	ratings      []domain.Rating
	note         string
	noteDeleted  bool
	ratingUserID domain.UserID
}

func (s *stubReviewRepo) Ratings(context.Context, domain.Actor, domain.ItemID) ([]domain.Rating, error) {
	return s.ratings, nil
}

func (s *stubReviewRepo) SetRating(_ context.Context, _ domain.Actor, itemID domain.ItemID, userID domain.UserID, score int) (domain.Rating, error) {
	s.ratingUserID = userID
	r := domain.Rating{ItemID: itemID, UserID: userID, Score: score}
	s.ratings = append(s.ratings, r)
	return r, nil
}

func (s *stubReviewRepo) Note(context.Context, domain.Actor, domain.ItemID) (domain.Note, error) {
	return domain.Note{Body: s.note}, nil
}

func (s *stubReviewRepo) SetNote(_ context.Context, _ domain.Actor, itemID domain.ItemID, body string) (domain.Note, error) {
	s.note = body
	return domain.Note{ItemID: itemID, Body: body}, nil
}

func (s *stubReviewRepo) DeleteNote(context.Context, domain.Actor, domain.ItemID) error {
	s.note = ""
	s.noteDeleted = true
	return nil
}

// TestReviewValidation covers the service-level shape checks.
func TestReviewValidation(t *testing.T) {
	actor := domain.NewActor(2)
	svc := NewReviews(&stubReviewRepo{})

	cases := []struct {
		name    string
		run     func() error
		wantErr error
	}{
		{"no actor on rating", func() error {
			return svc.SetRating(context.Background(), domain.Actor{}, 1, 3)
		}, domain.ErrNoActor},
		{"zero item", func() error {
			return svc.SetRating(context.Background(), actor, 0, 3)
		}, domain.ErrValidation},
		{"score too low", func() error {
			return svc.SetRating(context.Background(), actor, 1, 0)
		}, domain.ErrValidation},
		{"score too high", func() error {
			return svc.SetRating(context.Background(), actor, 1, 6)
		}, domain.ErrValidation},
		{"note too long", func() error {
			return svc.SetNote(context.Background(), actor, 1, strings.Repeat("x", maxNoteLength+1))
		}, domain.ErrValidation},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.run(); !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
		})
	}
}

// TestSetRatingUsesActorIdentity proves the written user id is always the
// acting user.
func TestSetRatingUsesActorIdentity(t *testing.T) {
	repo := &stubReviewRepo{}
	svc := NewReviews(repo)
	if err := svc.SetRating(context.Background(), domain.NewActor(2), 7, 5); err != nil {
		t.Fatalf("SetRating: %v", err)
	}
	if repo.ratingUserID != 2 {
		t.Fatalf("rating user = %d, want 2", repo.ratingUserID)
	}
}

// TestSetNoteEmptyClears proves a whitespace-only body deletes the note.
func TestSetNoteEmptyClears(t *testing.T) {
	repo := &stubReviewRepo{note: "vieja"}
	svc := NewReviews(repo)
	if err := svc.SetNote(context.Background(), domain.NewActor(1), 7, "   "); err != nil {
		t.Fatalf("SetNote: %v", err)
	}
	if !repo.noteDeleted {
		t.Fatal("empty note did not clear the stored note")
	}
}
