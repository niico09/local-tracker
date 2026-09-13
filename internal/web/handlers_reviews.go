package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"local-tracker/internal/domain"
)

// Reviews is the reviews use-case surface the HTTP layer depends on.
type Reviews interface {
	Get(ctx context.Context, a domain.Actor, itemID domain.ItemID) ([]domain.Rating, domain.Note, error)
	SetRating(ctx context.Context, a domain.Actor, itemID domain.ItemID, score int) error
	SetNote(ctx context.Context, a domain.Actor, itemID domain.ItemID, body string) error
}

// handleRatingSet stores the acting user's score and returns to the item.
func handleRatingSet(reviews Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := itemID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		score, err := strconv.Atoi(strings.TrimSpace(r.FormValue("score")))
		if err != nil {
			fail(w, r, fmt.Errorf("%w: score is required", domain.ErrValidation))
			return
		}
		if err := reviews.SetRating(r.Context(), actorOf(r), id, score); err != nil {
			fail(w, r, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/catalog/%d", id), http.StatusSeeOther)
	}
}

// handleNoteSet upserts the shared note and returns to the item. An empty body
// clears the note.
func handleNoteSet(reviews Reviews) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := itemID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		if err := reviews.SetNote(r.Context(), actorOf(r), id, r.FormValue("body")); err != nil {
			fail(w, r, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/catalog/%d", id), http.StatusSeeOther)
	}
}
