package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"local-tracker/internal/domain"
)

// Membership is the goal-membership use-case surface the HTTP layer depends on.
type Membership interface {
	ListMembers(ctx context.Context, a domain.Actor, goalID domain.GoalID) ([]domain.GoalMember, error)
	Add(ctx context.Context, a domain.Actor, goalID domain.GoalID, itemID domain.ItemID) error
	Remove(ctx context.Context, a domain.Actor, goalID domain.GoalID, itemID domain.ItemID) error
}

// memberView is one item inside a goal plus the Rule 1 tilt owner and derived
// tilt state the goal page displays.
type memberView struct {
	ID        domain.ItemID
	Title     string
	Kind      domain.Kind
	Year      *int
	CoverURL  string
	TiltOwner domain.UserID // 0 = shared couple tilt
	TiltState domain.State
	HasTilt   bool
}

// toMemberView decorates a goal member for the template. User data is only ever
// rendered through html/template's default escaping.
func toMemberView(m domain.GoalMember) memberView {
	v := memberView{
		ID:        m.ID,
		Title:     m.Title,
		Kind:      m.Kind,
		Year:      m.Year,
		TiltOwner: m.TiltOwner,
	}
	if m.CoverPath != "" {
		v.CoverURL = "/uploads/" + m.CoverPath
	}
	if m.Tilt != nil {
		v.HasTilt = true
		v.TiltState = domain.DeriveState(*m.Tilt)
	}
	return v
}

// memberItemID parses the {itemID} wildcard of a membership route.
func memberItemID(r *http.Request) (domain.ItemID, bool) {
	id, err := strconv.ParseInt(r.PathValue("itemID"), 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return domain.ItemID(id), true
}

// handleMembershipAdd attaches a catalog item to a goal. Form value item_id is
// validated here; goal visibility and item visibility are enforced downstream.
func handleMembershipAdd(members Membership) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		goal, ok := goalID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		raw := strings.TrimSpace(r.FormValue("item_id"))
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			fail(w, r, fmt.Errorf("%w: item is required", domain.ErrValidation))
			return
		}
		if err := members.Add(r.Context(), actorOf(r), goal, domain.ItemID(id)); err != nil {
			fail(w, r, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/goals/%d", goal), http.StatusSeeOther)
	}
}

// handleMembershipRemove detaches an item from a goal without deleting the item
// or its progress.
func handleMembershipRemove(members Membership) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		goal, ok := goalID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		item, ok := memberItemID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		if err := members.Remove(r.Context(), actorOf(r), goal, item); err != nil {
			fail(w, r, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/goals/%d", goal), http.StatusSeeOther)
	}
}
