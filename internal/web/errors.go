package web

import (
	"errors"
	"log"
	"net/http"

	"local-tracker/internal/domain"
)

// fail maps domain errors to HTTP responses. Hidden resources and forbidden
// writes both become 404 so a private goal never leaks its existence.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrForbidden), errors.Is(err, domain.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, domain.ErrValidation):
		http.Error(w, "invalid input", http.StatusUnprocessableEntity)
	case errors.Is(err, domain.ErrNoActor):
		http.Redirect(w, r, "/login", http.StatusFound)
	default:
		log.Printf("request error: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
