package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"local-tracker/internal/domain"
	"local-tracker/internal/service"
)

// Goals is the goals use-case surface the HTTP layer depends on.
type Goals interface {
	List(ctx context.Context, a domain.Actor) ([]domain.Goal, error)
	Get(ctx context.Context, a domain.Actor, id domain.GoalID) (domain.Goal, error)
	Create(ctx context.Context, a domain.Actor, in service.GoalInput) (domain.Goal, error)
	Update(ctx context.Context, a domain.Actor, id domain.GoalID, in service.GoalInput) (domain.Goal, error)
	SetVisibility(ctx context.Context, a domain.Actor, id domain.GoalID, v domain.Visibility) error
	Delete(ctx context.Context, a domain.Actor, id domain.GoalID) error
}

// goalView is a goal plus the browser-facing edit affordance. Editable is
// derived from the single access rule, never re-implemented here.
type goalView struct {
	domain.Goal
	Editable bool
}

// goalForm keeps submitted values so a rejected form re-renders with input.
type goalForm struct {
	Title      string
	Target     string
	Couple     bool
	Visibility domain.Visibility
}

// toGoalView decorates a goal with the acting user's modify permission.
func toGoalView(a domain.Actor, g domain.Goal) goalView {
	return goalView{
		Goal:     g,
		Editable: domain.Authorize(a, domain.ActionModify, g.Subject()) == nil,
	}
}

func goalFormFrom(r *http.Request) goalForm {
	return goalForm{
		Title:      r.FormValue("title"),
		Target:     r.FormValue("target"),
		Couple:     r.FormValue("couple") != "",
		Visibility: domain.Visibility(r.FormValue("visibility")),
	}
}

// goalID parses the {id} wildcard, rejecting anything that is not a positive id.
func goalID(r *http.Request) (domain.GoalID, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return domain.GoalID(id), true
}

// parseTarget accepts an optional strictly positive integer target.
func parseTarget(raw string) (*int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: target must be a whole number", domain.ErrValidation)
	}
	if n <= 0 {
		return nil, fmt.Errorf("%w: target must be greater than zero", domain.ErrValidation)
	}
	return &n, nil
}

// goalInput converts a submitted form into a service input. A personal goal is
// owned by the actor; a couple goal carries owner 0.
func goalInput(a domain.Actor, form goalForm, target *int) service.GoalInput {
	owner := domain.UserID(0)
	if !form.Couple {
		owner = a.ID()
	}
	visibility := form.Visibility
	if visibility == "" {
		visibility = domain.VisibilityPrivate
	}
	return service.GoalInput{Title: form.Title, OwnerUserID: owner, Target: target, Visibility: visibility}
}

func handleGoalList(goals Goals, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a := actorOf(r)
		list, err := goals.List(r.Context(), a)
		if err != nil {
			fail(w, r, err)
			return
		}
		views := make([]goalView, 0, len(list))
		for _, g := range list {
			views = append(views, toGoalView(a, g))
		}
		render(w, tmpls, "goals_list", http.StatusOK, pageData{Title: "Goals", Goals: views})
	}
}

func handleGoalNew(tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		render(w, tmpls, "goal_new", http.StatusOK,
			pageData{Title: "Add goal", Visibilities: domain.Visibilities(),
				GoalForm: goalForm{Visibility: domain.VisibilityPrivate}})
	}
}

func handleGoalCreate(goals Goals, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		form := goalFormFrom(r)
		target, err := parseTarget(form.Target)
		if err != nil {
			render(w, tmpls, "goal_new", http.StatusUnprocessableEntity, pageData{
				Title: "Add goal", Error: err.Error(), Visibilities: domain.Visibilities(), GoalForm: form,
			})
			return
		}
		a := actorOf(r)
		goal, err := goals.Create(r.Context(), a, goalInput(a, form, target))
		if err != nil {
			if errors.Is(err, domain.ErrValidation) {
				render(w, tmpls, "goal_new", http.StatusUnprocessableEntity, pageData{
					Title: "Add goal", Error: err.Error(), Visibilities: domain.Visibilities(), GoalForm: form,
				})
				return
			}
			fail(w, r, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/goals/%d", goal.ID), http.StatusSeeOther)
	}
}

func handleGoalDetail(goals Goals, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := goalID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		a := actorOf(r)
		goal, err := goals.Get(r.Context(), a, id)
		if err != nil {
			fail(w, r, err)
			return
		}
		v := toGoalView(a, goal)
		render(w, tmpls, "goal_detail", http.StatusOK, pageData{Title: goal.Title, Goal: &v})
	}
}

func handleGoalEdit(goals Goals, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := goalID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		a := actorOf(r)
		goal, err := goals.Get(r.Context(), a, id)
		if err != nil {
			fail(w, r, err)
			return
		}
		// A granted partner may view but never edit; surface the same 404 as a
		// hidden goal so the edit form never appears.
		if domain.Authorize(a, domain.ActionModify, goal.Subject()) != nil {
			fail(w, r, domain.ErrForbidden)
			return
		}
		form := goalForm{Title: goal.Title, Couple: goal.OwnerUserID == 0, Visibility: goal.Visibility}
		if goal.Target != nil {
			form.Target = strconv.Itoa(*goal.Target)
		}
		v := toGoalView(a, goal)
		render(w, tmpls, "goal_edit", http.StatusOK,
			pageData{Title: "Edit " + goal.Title, Goal: &v, Visibilities: domain.Visibilities(), GoalForm: form})
	}
}

func handleGoalUpdate(goals Goals, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := goalID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		form := goalFormFrom(r)
		target, err := parseTarget(form.Target)
		if err != nil {
			render(w, tmpls, "goal_edit", http.StatusUnprocessableEntity, pageData{
				Title: "Edit goal", Error: err.Error(), Visibilities: domain.Visibilities(), GoalForm: form,
			})
			return
		}
		a := actorOf(r)
		goal, err := goals.Update(r.Context(), a, id, goalInput(a, form, target))
		if err != nil {
			if errors.Is(err, domain.ErrValidation) {
				render(w, tmpls, "goal_edit", http.StatusUnprocessableEntity, pageData{
					Title: "Edit goal", Error: err.Error(), Visibilities: domain.Visibilities(), GoalForm: form,
				})
				return
			}
			fail(w, r, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/goals/%d", goal.ID), http.StatusSeeOther)
	}
}

// handleGoalVisibility grants or revokes partner visibility. It defaults to
// granting ('shared') when no value is submitted.
func handleGoalVisibility(goals Goals) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := goalID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		v := domain.Visibility(r.FormValue("visibility"))
		if v == "" {
			v = domain.VisibilityShared
		}
		if err := goals.SetVisibility(r.Context(), actorOf(r), id, v); err != nil {
			fail(w, r, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/goals/%d", id), http.StatusSeeOther)
	}
}

func handleGoalDelete(goals Goals) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := goalID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		if err := goals.Delete(r.Context(), actorOf(r), id); err != nil {
			fail(w, r, err)
			return
		}
		http.Redirect(w, r, "/goals", http.StatusSeeOther)
	}
}
