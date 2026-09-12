package web_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"local-tracker/internal/domain"
)

// loginAs opens a fresh cookie jar, logs in the named profile and returns a
// client scoped to that session so a test can act as either partner.
func loginAs(t *testing.T, h *harness, name, pin string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	client := &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp := doAs(t, client, h, http.MethodPost, "/login", loginForm(name, pin))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login %s = %d, want 303", name, resp.StatusCode)
	}
	return client
}

// doAs issues a form request through a specific session client.
func doAs(t *testing.T, client *http.Client, h *harness, method, path string, form url.Values) *http.Response {
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
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// createGoal posts a goal and returns its detail path.
func createGoal(t *testing.T, h *harness, form url.Values) string {
	t.Helper()
	resp := h.do(t, http.MethodPost, "/goals", form)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /goals = %d, want 303", resp.StatusCode)
	}
	location := resp.Header.Get("Location")
	if !strings.HasPrefix(location, "/goals/") {
		t.Fatalf("create Location = %q, want /goals/{id}", location)
	}
	return location
}

func goalIDFromPath(t *testing.T, path string) domain.GoalID {
	t.Helper()
	id, err := strconv.ParseInt(strings.TrimPrefix(path, "/goals/"), 10, 64)
	if err != nil {
		t.Fatalf("bad goal path %q: %v", path, err)
	}
	return domain.GoalID(id)
}

// TestGoalPrivateHiddenFromPartner proves a private personal goal returns 404 on
// a direct partner fetch and never appears in the partner's list.
func TestGoalPrivateHiddenFromPartner(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)
	detail := createGoal(t, h, url.Values{"title": {"Ada secret"}})

	if resp := h.do(t, http.MethodGet, detail, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("owner GET %s = %d, want 200", detail, resp.StatusCode)
	} else {
		resp.Body.Close()
	}

	partner := loginAs(t, h, "Linus", "5678")
	resp := doAs(t, partner, h, http.MethodGet, detail, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("partner GET %s = %d, want 404", detail, resp.StatusCode)
	}
	resp.Body.Close()

	listResp := doAs(t, partner, h, http.MethodGet, "/goals", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("partner GET /goals = %d, want 200", listResp.StatusCode)
	}
	if body := readBody(t, listResp); strings.Contains(body, "Ada secret") {
		t.Fatalf("private goal leaked into partner list: %s", body)
	}
}

// TestGoalGrantReadOnlyForPartner proves a granted shared goal is readable by the
// partner while every partner write is rejected.
func TestGoalGrantReadOnlyForPartner(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)
	detail := createGoal(t, h, url.Values{"title": {"Shared goal"}})

	grant := h.do(t, http.MethodPost, detail+"/visibility", url.Values{"visibility": {"shared"}})
	if grant.StatusCode != http.StatusSeeOther {
		t.Fatalf("grant = %d, want 303", grant.StatusCode)
	}
	grant.Body.Close()

	partner := loginAs(t, h, "Linus", "5678")

	resp := doAs(t, partner, h, http.MethodGet, detail, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("granted partner GET = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	writes := []struct {
		name string
		path string
		form url.Values
	}{
		{"edit", detail + "/edit", url.Values{"title": {"stolen"}}},
		{"delete", detail + "/delete", nil},
		{"visibility", detail + "/visibility", url.Values{"visibility": {"private"}}},
	}
	for _, tc := range writes {
		resp := doAs(t, partner, h, http.MethodPost, tc.path, tc.form)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("partner %s = %d, want 404", tc.name, resp.StatusCode)
		}
		resp.Body.Close()
	}

	stored, err := h.repos.Goals.Get(context.Background(), domain.NewActor(1), goalIDFromPath(t, detail))
	if err != nil || stored.Title != "Shared goal" {
		t.Fatalf("stored goal = %+v, %v; want unchanged", stored, err)
	}
}

// TestGoalCoupleVisibleToBoth proves a couple goal is shared and mutable by both.
func TestGoalCoupleVisibleToBoth(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)
	detail := createGoal(t, h, url.Values{"title": {"Couple goal"}, "couple": {"on"}})

	partner := loginAs(t, h, "Linus", "5678")
	resp := doAs(t, partner, h, http.MethodGet, detail, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("partner couple GET = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doAs(t, partner, h, http.MethodPost, detail+"/edit", url.Values{"title": {"Couple v2"}, "couple": {"on"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("partner couple edit = %d, want 303", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestGoalNewBeatsIDWildcard proves the literal /goals/new route wins over
// /goals/{id} and registration does not panic.
func TestGoalNewBeatsIDWildcard(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	resp := h.do(t, http.MethodGet, "/goals/new", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /goals/new = %d, want 200", resp.StatusCode)
	}
	if body := readBody(t, resp); !strings.Contains(body, "Add goal") {
		t.Fatalf("unexpected /goals/new body: %s", body)
	}
}

// TestGoalCreateRejectsNonPositiveTarget proves the target invariant returns 422.
func TestGoalCreateRejectsNonPositiveTarget(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	for _, target := range []string{"0", "-4", "abc"} {
		resp := h.do(t, http.MethodPost, "/goals", url.Values{"title": {"Bad"}, "target": {target}})
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("target %q = %d, want 422", target, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
