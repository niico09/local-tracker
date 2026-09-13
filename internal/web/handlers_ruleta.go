package web

import (
	"math/rand/v2"
	"net/http"

	"local-tracker/internal/domain"
)

// ruletaPick is one spun result: the item plus its resolved standalone state.
type ruletaPick struct {
	itemView
	State domain.State
}

// handleRuleta spins a random not-yet-completed item of the chosen kind
// (series or movies). Without a kind it renders the category chooser. Random
// selection lives here because it is presentation, not a domain rule.
func handleRuleta(cat Catalog, progress Progress, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := actorOf(r)
		items, err := cat.List(r.Context(), a)
		if err != nil {
			fail(w, r, err)
			return
		}

		kind := domain.Kind(r.URL.Query().Get("kind"))
		pickKind := kind == domain.KindSeries || kind == domain.KindMovie
		data := pageData{Title: "Ruleta"}
		var candidates []ruletaPick

		for _, it := range items {
			switch it.Kind {
			case domain.KindSeries:
				data.SeriesCount++
			case domain.KindMovie:
				data.MovieCount++
			}
			if !pickKind || it.Kind != kind {
				continue
			}
			tilt, err := progress.Tilt(r.Context(), a, 0, it.ID)
			if err != nil {
				fail(w, r, err)
				return
			}
			state := domain.DeriveState(tilt)
			if state == domain.StateCompleted {
				continue
			}
			candidates = append(candidates, ruletaPick{itemView: toView(it), State: state})
		}

		if pickKind {
			data.Kind = kind
			if len(candidates) > 0 {
				pick := candidates[rand.IntN(len(candidates))]
				data.Pick = &pick
			}
		}
		render(w, tmpls, "ruleta", http.StatusOK, data)
	}
}
