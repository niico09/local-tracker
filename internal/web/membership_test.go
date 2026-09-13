package web_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// itemIDFromPath extracts the id from a created item's /catalog/{id} path.
func itemIDFromPath(t *testing.T, path string) string {
	t.Helper()
	id := strings.TrimPrefix(path, "/catalog/")
	if id == "" || id == path {
		t.Fatalf("bad item path %q", path)
	}
	return id
}

// TestGoalMembershipAddRemoveRoundTrip proves the goal page can attach and
// detach a catalog item and that the resolved tilt is shown as shared for a
// couple goal.
func TestGoalMembershipAddRemoveRoundTrip(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	itemPath := createItem(t, h, "Dune")
	itemID := itemIDFromPath(t, itemPath)
	goalPath := createGoal(t, h, url.Values{"title": {"Couple"}, "couple": {"on"}})
	removeAction := goalPath + "/items/" + itemID + "/remove"

	add := h.do(t, http.MethodPost, goalPath+"/items", url.Values{"item_id": {itemID}})
	if add.StatusCode != http.StatusSeeOther {
		t.Fatalf("add membership = %d, want 303", add.StatusCode)
	}
	add.Body.Close()

	body := readBody(t, h.do(t, http.MethodGet, goalPath, nil))
	if !strings.Contains(body, "Dune") {
		t.Fatalf("goal detail missing member item: %s", body)
	}
	if !strings.Contains(body, `data-owner="shared"`) {
		t.Fatalf("goal detail missing shared tilt: %s", body)
	}
	if !strings.Contains(body, removeAction) {
		t.Fatalf("goal detail missing remove affordance for an editable goal: %s", body)
	}

	rm := h.do(t, http.MethodPost, removeAction, nil)
	if rm.StatusCode != http.StatusSeeOther {
		t.Fatalf("remove membership = %d, want 303", rm.StatusCode)
	}
	rm.Body.Close()

	body = readBody(t, h.do(t, http.MethodGet, goalPath, nil))
	if strings.Contains(body, removeAction) {
		t.Fatalf("membership survived removal: %s", body)
	}
}

// TestGoalMembershipReadOnlyPartner proves a partner granted view of a personal
// goal can see its members but may not add or remove them.
func TestGoalMembershipReadOnlyPartner(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	itemPath := createItem(t, h, "Dune")
	itemID := itemIDFromPath(t, itemPath)
	goalPath := createGoal(t, h, url.Values{"title": {"Shared goal"}})

	if resp := h.do(t, http.MethodPost, goalPath+"/visibility", url.Values{"visibility": {"shared"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("grant = %d, want 303", resp.StatusCode)
	}
	if resp := h.do(t, http.MethodPost, goalPath+"/items", url.Values{"item_id": {itemID}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("owner add = %d, want 303", resp.StatusCode)
	}

	partner := loginAs(t, h, "Linus", "5678")

	resp := doAs(t, partner, h, http.MethodGet, goalPath, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("granted partner GET = %d, want 200", resp.StatusCode)
	}
	body := readBody(t, resp)
	if !strings.Contains(body, "Dune") || !strings.Contains(body, `data-owner="personal"`) {
		t.Fatalf("granted partner cannot see members: %s", body)
	}
	if strings.Contains(body, "/remove") {
		t.Fatalf("read-only partner was offered a remove affordance: %s", body)
	}

	add := doAs(t, partner, h, http.MethodPost, goalPath+"/items", url.Values{"item_id": {itemID}})
	if add.StatusCode != http.StatusNotFound {
		t.Fatalf("partner add = %d, want 404", add.StatusCode)
	}
	add.Body.Close()

	rm := doAs(t, partner, h, http.MethodPost, goalPath+"/items/"+itemID+"/remove", nil)
	if rm.StatusCode != http.StatusNotFound {
		t.Fatalf("partner remove = %d, want 404", rm.StatusCode)
	}
	rm.Body.Close()
}
