package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"local-tracker/internal/domain"
)

// Progress is the progress use-case surface the HTTP layer depends on.
type Progress interface {
	Tilt(ctx context.Context, a domain.Actor, goalID domain.GoalID, itemID domain.ItemID) (domain.Progress, error)
	Toggle(ctx context.Context, a domain.Actor, goalID domain.GoalID, itemID domain.ItemID) (domain.Progress, error)
	SetDates(ctx context.Context, a domain.Actor, goalID domain.GoalID, itemID domain.ItemID, done bool, start, end *domain.Date) (domain.Progress, error)
	GoalRollup(ctx context.Context, a domain.Actor, goalID domain.GoalID) (done, total int, err error)
}

// rollupView is the display pair a goal progress fragment renders.
type rollupView struct {
	Done  int
	Total int
}

// tiltView decorates one tilt for the templates. Owner 0 is the shared couple
// tilt; dates are rendered in the storage layout.
type tiltView struct {
	ItemID    domain.ItemID
	Owner     domain.UserID
	State     domain.State
	Done      bool
	StartDate string
	EndDate   string
}

// toTiltView decorates a progress row with its derived state.
func toTiltView(p domain.Progress) tiltView {
	v := tiltView{ItemID: p.ItemID, Owner: p.OwnerUserID, State: domain.DeriveState(p), Done: p.Done}
	if p.StartDate != nil {
		v.StartDate = p.StartDate.String()
	}
	if p.EndDate != nil {
		v.EndDate = p.EndDate.String()
	}
	return v
}

// tiltViewPtr returns a heap tilt view for the template payload.
func tiltViewPtr(p domain.Progress) *tiltView {
	v := toTiltView(p)
	return &v
}

// parseGoalIDForm reads the optional goal_id form value. Empty means the
// standalone tilt; a present value must be a positive id.
func parseGoalIDForm(r *http.Request) (domain.GoalID, error) {
	raw := strings.TrimSpace(r.FormValue("goal_id"))
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: invalid goal", domain.ErrValidation)
	}
	return domain.GoalID(id), nil
}

// parseOptionalDate parses a 'YYYY-MM-DD' form value; empty is nil.
func parseOptionalDate(raw string) (*domain.Date, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(domain.DateLayout, raw)
	if err != nil {
		return nil, fmt.Errorf("%w: dates must be YYYY-MM-DD", domain.ErrValidation)
	}
	d := domain.Date{Time: t}
	return &d, nil
}

// progressItemID parses the {itemID} wildcard of a progress route.
func progressItemID(r *http.Request) (domain.ItemID, bool) {
	id, err := strconv.ParseInt(r.PathValue("itemID"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return domain.ItemID(id), true
}

// handleProgressToggle flips the resolved tilt. HTMX requests receive the
// refreshed fragment; plain requests are redirected (303) to the goal or the
// catalog item so the app degrades without JavaScript.
func handleProgressToggle(progress Progress, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, ok := progressItemID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		goal, err := parseGoalIDForm(r)
		if err != nil {
			fail(w, r, err)
			return
		}
		tilt, err := progress.Toggle(r.Context(), actorOf(r), goal, item)
		if err != nil {
			fail(w, r, err)
			return
		}
		respondTilt(w, r, progress, tmpls, goal, item, tilt)
	}
}

// handleProgressDates stores the optional start/end dates plus the done flag.
func handleProgressDates(progress Progress, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		item, ok := progressItemID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		goal, err := parseGoalIDForm(r)
		if err != nil {
			fail(w, r, err)
			return
		}
		start, err := parseOptionalDate(r.FormValue("start"))
		if err != nil {
			fail(w, r, err)
			return
		}
		end, err := parseOptionalDate(r.FormValue("end"))
		if err != nil {
			fail(w, r, err)
			return
		}
		tilt, err := progress.SetDates(r.Context(), actorOf(r), goal, item, r.FormValue("done") != "", start, end)
		if err != nil {
			fail(w, r, err)
			return
		}
		respondTilt(w, r, progress, tmpls, goal, item, tilt)
	}
}

// respondTilt renders the refreshed fragment for an HTMX mutation, or issues the
// non-HTMX 303 redirect to the page that owns the tilt.
func respondTilt(w http.ResponseWriter, r *http.Request, progress Progress, tmpls templates, goal domain.GoalID, item domain.ItemID, tilt domain.Progress) {
	if r.Header.Get("HX-Request") == "true" {
		if goal > 0 {
			done, total, err := progress.GoalRollup(r.Context(), actorOf(r), goal)
			if err != nil {
				fail(w, r, err)
				return
			}
			renderFragment(w, r, tmpls, "goal_progress", "progress_goal", http.StatusOK, pageData{Rollup: &rollupView{Done: done, Total: total}})
			return
		}
		renderFragment(w, r, tmpls, "catalog_tilt", "progress_tilt", http.StatusOK, pageData{Tilt: tiltViewPtr(tilt)})
		return
	}
	if goal > 0 {
		http.Redirect(w, r, fmt.Sprintf("/goals/%d", goal), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/catalog/%d", item), http.StatusSeeOther)
}

// handleGoalProgress is the goal rollup fragment. HTMX receives only the
// fragment; a plain request gets the full page around it.
func handleGoalProgress(progress Progress, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := goalID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		done, total, err := progress.GoalRollup(r.Context(), actorOf(r), id)
		if err != nil {
			fail(w, r, err)
			return
		}
		renderFragment(w, r, tmpls, "goal_progress", "progress_goal", http.StatusOK, pageData{
			Title: "Progress", Rollup: &rollupView{Done: done, Total: total},
		})
	}
}

// handleCatalogTilt is the standalone tilt fragment (owner = item owner, G5).
func handleCatalogTilt(progress Progress, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := itemID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		tilt, err := progress.Tilt(r.Context(), actorOf(r), 0, id)
		if err != nil {
			fail(w, r, err)
			return
		}
		renderFragment(w, r, tmpls, "catalog_tilt", "progress_tilt", http.StatusOK, pageData{
			Title: "Tilt", Tilt: tiltViewPtr(tilt),
		})
	}
}
