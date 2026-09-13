package web_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// doRequest issues a form request through one session client, optionally
// declaring an HTMX request so the fragment-only branch is exercised.
func doRequest(t *testing.T, client *http.Client, h *harness, method, path string, form url.Values, htmx bool) *http.Response {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, h.srv.URL+path, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// addItemToGoal attaches a catalog item to a goal through the membership route.
func addItemToGoal(t *testing.T, h *harness, goalPath, itemID string) {
	t.Helper()
	resp := h.do(t, http.MethodPost, goalPath+"/items", url.Values{"item_id": {itemID}})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("add item to goal = %d, want 303", resp.StatusCode)
	}
}

// TestProgressToggleHTMXAndNonHTMX proves an HTMX mutation returns the refreshed
// fragment while a plain request gets a 303 redirect, and that the fragment
// carries no full document.
func TestProgressToggleHTMXAndNonHTMX(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	itemID := itemIDFromPath(t, createItem(t, h, "Dune"))
	goalPath := createGoal(t, h, url.Values{"title": {"Couple"}, "couple": {"on"}})
	addItemToGoal(t, h, goalPath, itemID)
	toggle := "/progress/" + itemID + "/toggle"
	goalID := strings.TrimPrefix(goalPath, "/goals/")

	body := readBody(t, doRequest(t, h.client, h, http.MethodPost, toggle, url.Values{"goal_id": {goalID}}, true))
	if !strings.Contains(body, `id="goal-progress"`) || !strings.Contains(body, "1/1") {
		t.Fatalf("HTMX toggle fragment = %q, want the refreshed 1/1 rollup", body)
	}
	if strings.Contains(body, "<html") {
		t.Fatalf("HTMX toggle returned a full document: %q", body)
	}

	resp := doRequest(t, h.client, h, http.MethodPost, toggle, url.Values{"goal_id": {goalID}}, false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != goalPath {
		t.Fatalf("plain toggle = %d %q, want 303 %s", resp.StatusCode, resp.Header.Get("Location"), goalPath)
	}
}

// TestProgressStandaloneToggleDefaultsShared covers Rule 4: an item outside
// every goal toggles the shared tilt, and the other partner can do the same.
func TestProgressStandaloneToggleDefaultsShared(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	itemID := itemIDFromPath(t, createItem(t, h, "Dune"))
	partner := loginAs(t, h, "Linus", "5678")

	body := readBody(t, doRequest(t, partner, h, http.MethodPost, "/progress/"+itemID+"/toggle", nil, true))
	if !strings.Contains(body, `data-state="completed"`) || !strings.Contains(body, `data-owner="shared"`) {
		t.Fatalf("standalone tilt fragment = %q, want completed (shared)", body)
	}
	if strings.Contains(body, "<html") {
		t.Fatalf("HTMX tilt fragment returned a full document: %q", body)
	}
}

// TestProgressDatesDoneNoDatesAndEndBeforeStart covers Rule 5: done with no
// dates reads completed, and an end before the start is rejected with 422.
func TestProgressDatesDoneNoDatesAndEndBeforeStart(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	itemID := itemIDFromPath(t, createItem(t, h, "Dune"))
	dates := "/progress/" + itemID + "/dates"

	body := readBody(t, doRequest(t, h.client, h, http.MethodPost, dates, url.Values{"done": {"on"}}, true))
	if !strings.Contains(body, `data-state="completed"`) {
		t.Fatalf("done-without-dates fragment = %q, want completed", body)
	}

	resp := doRequest(t, h.client, h, http.MethodPost, dates,
		url.Values{"start": {"2024-02-01"}, "end": {"2024-01-01"}}, false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("end<start = %d, want 422", resp.StatusCode)
	}
}

// TestGoalProgressFragmentAndFullPage proves the GET endpoint serves a fragment
// to HTMX and the full page otherwise, and that a target drives the denominator.
func TestGoalProgressFragmentAndFullPage(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	itemID := itemIDFromPath(t, createItem(t, h, "Dune"))
	goalPath := createGoal(t, h, url.Values{"title": {"Top 100"}, "couple": {"on"}, "target": {"100"}})
	addItemToGoal(t, h, goalPath, itemID)
	progressPath := goalPath + "/progress"

	body := readBody(t, doRequest(t, h.client, h, http.MethodGet, progressPath, nil, true))
	if !strings.Contains(body, "0/100") || strings.Contains(body, "<html") {
		t.Fatalf("goal progress fragment = %q, want 0/100 without a full document", body)
	}

	full := readBody(t, doRequest(t, h.client, h, http.MethodGet, progressPath, nil, false))
	if !strings.Contains(full, "0/100") || !strings.Contains(full, "<html") {
		t.Fatalf("goal progress full page = %q, want 0/100 inside a document", full)
	}
}

// TestCatalogTiltFragmentAndFullPage proves the standalone tilt endpoint serves
// a pending/shared fragment to HTMX and the full page otherwise.
func TestCatalogTiltFragmentAndFullPage(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	itemID := itemIDFromPath(t, createItem(t, h, "Dune"))
	tiltPath := "/catalog/" + itemID + "/tilt"

	body := readBody(t, doRequest(t, h.client, h, http.MethodGet, tiltPath, nil, true))
	if !strings.Contains(body, `data-state="pending"`) || !strings.Contains(body, `data-owner="shared"`) || strings.Contains(body, "<html") {
		t.Fatalf("catalog tilt fragment = %q, want pending (shared) only", body)
	}

	full := readBody(t, doRequest(t, h.client, h, http.MethodGet, tiltPath, nil, false))
	if !strings.Contains(full, "<html") || !strings.Contains(full, `data-state="pending"`) {
		t.Fatalf("catalog tilt full page = %q, want the fragment inside a document", full)
	}
}

// TestTop100SharedAdvance covers Rule 2: a couple goal with target 100 shows one
// shared advance for both partners.
func TestTop100SharedAdvance(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	itemID := itemIDFromPath(t, createItem(t, h, "Dune"))
	goalPath := createGoal(t, h, url.Values{"title": {"Top 100"}, "couple": {"on"}, "target": {"100"}})
	addItemToGoal(t, h, goalPath, itemID)
	progressPath := goalPath + "/progress"
	goalID := strings.TrimPrefix(goalPath, "/goals/")

	if resp := doRequest(t, h.client, h, http.MethodPost, "/progress/"+itemID+"/toggle", url.Values{"goal_id": {goalID}}, false); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("owner toggle = %d, want 303", resp.StatusCode)
	}

	partner := loginAs(t, h, "Linus", "5678")
	if body := readBody(t, doRequest(t, partner, h, http.MethodGet, progressPath, nil, true)); !strings.Contains(body, "1/100") {
		t.Fatalf("partner rollup = %q, want the shared 1/100", body)
	}

	// The partner toggles the same shared advance back off.
	if resp := doRequest(t, partner, h, http.MethodPost, "/progress/"+itemID+"/toggle", url.Values{"goal_id": {goalID}}, false); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("partner toggle = %d, want 303", resp.StatusCode)
	}
	if body := readBody(t, doRequest(t, h.client, h, http.MethodGet, progressPath, nil, true)); !strings.Contains(body, "0/100") {
		t.Fatalf("owner rollup after partner toggle = %q, want the shared 0/100", body)
	}
}

// TestGoalProgressHiddenFromPartner proves a private personal goal's rollup is
// a 404 for the partner.
func TestGoalProgressHiddenFromPartner(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	itemID := itemIDFromPath(t, createItem(t, h, "Dune"))
	goalPath := createGoal(t, h, url.Values{"title": {"Ada secret"}})
	addItemToGoal(t, h, goalPath, itemID)

	partner := loginAs(t, h, "Linus", "5678")
	resp := doRequest(t, partner, h, http.MethodGet, goalPath+"/progress", nil, true)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("partner goal rollup = %d, want 404", resp.StatusCode)
	}
}

// TestGoalProgressGrantedPartnerReadOnly proves the grant allows a partner to
// read a personal goal's rollup but never to change its tilt.
func TestGoalProgressGrantedPartnerReadOnly(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	itemID := itemIDFromPath(t, createItem(t, h, "Dune"))
	goalPath := createGoal(t, h, url.Values{"title": {"Shared goal"}})
	goalID := strings.TrimPrefix(goalPath, "/goals/")
	if resp := h.do(t, http.MethodPost, goalPath+"/visibility", url.Values{"visibility": {"shared"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("grant = %d, want 303", resp.StatusCode)
	}
	addItemToGoal(t, h, goalPath, itemID)
	if resp := h.do(t, http.MethodPost, "/progress/"+itemID+"/toggle", url.Values{"goal_id": {goalID}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("owner toggle = %d, want 303", resp.StatusCode)
	}

	partner := loginAs(t, h, "Linus", "5678")
	if body := readBody(t, doRequest(t, partner, h, http.MethodGet, goalPath+"/progress", nil, true)); !strings.Contains(body, "1/1") {
		t.Fatalf("granted partner rollup = %q, want 1/1", body)
	}
	resp := doRequest(t, partner, h, http.MethodPost, "/progress/"+itemID+"/toggle", url.Values{"goal_id": {goalID}}, false)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("granted partner toggle = %d, want 404", resp.StatusCode)
	}
}
