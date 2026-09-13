package service

import (
	"context"
	"fmt"
	"strings"

	"local-tracker/internal/domain"
)

// maxNoteLength bounds the shared note so one paste cannot bloat the page.
const maxNoteLength = 2000

// ReviewService implements the ratings and shared-note use cases. Scores are
// always written for the acting user; the note belongs to the item and both
// partners may edit it.
type ReviewService struct {
	repo domain.ReviewRepository
}

// NewReviews wires the review use cases.
func NewReviews(repo domain.ReviewRepository) *ReviewService {
	return &ReviewService{repo: repo}
}

// Get returns the item's ratings plus its shared note (an empty note when none
// was ever written).
func (s *ReviewService) Get(ctx context.Context, a domain.Actor, itemID domain.ItemID) ([]domain.Rating, domain.Note, error) {
	if err := a.Require(); err != nil {
		return nil, domain.Note{}, err
	}
	if itemID <= 0 {
		return nil, domain.Note{}, fmt.Errorf("%w: item id is required", domain.ErrValidation)
	}
	ratings, err := s.repo.Ratings(ctx, a, itemID)
	if err != nil {
		return nil, domain.Note{}, err
	}
	note, err := s.repo.Note(ctx, a, itemID)
	if err != nil {
		return nil, domain.Note{}, err
	}
	return ratings, note, nil
}

// SetRating stores the acting user's score (1–5) on a visible item. The user id
// is never taken from input: a profile can only rate itself.
func (s *ReviewService) SetRating(ctx context.Context, a domain.Actor, itemID domain.ItemID, score int) error {
	if err := a.Require(); err != nil {
		return err
	}
	if itemID <= 0 {
		return fmt.Errorf("%w: item id is required", domain.ErrValidation)
	}
	if score < 1 || score > 5 {
		return fmt.Errorf("%w: score must be between 1 and 5", domain.ErrValidation)
	}
	_, err := s.repo.SetRating(ctx, a, itemID, a.ID(), score)
	return err
}

// SetNote upserts the shared note. An empty (or whitespace-only) body clears it.
func (s *ReviewService) SetNote(ctx context.Context, a domain.Actor, itemID domain.ItemID, body string) error {
	if err := a.Require(); err != nil {
		return err
	}
	if itemID <= 0 {
		return fmt.Errorf("%w: item id is required", domain.ErrValidation)
	}
	body = strings.TrimSpace(body)
	if len([]rune(body)) > maxNoteLength {
		return fmt.Errorf("%w: note must be at most %d characters", domain.ErrValidation, maxNoteLength)
	}
	if body == "" {
		return s.repo.DeleteNote(ctx, a, itemID)
	}
	_, err := s.repo.SetNote(ctx, a, itemID, body)
	return err
}
