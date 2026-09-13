package web_test

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestReviewsRoundTrip covers the rating and shared-note flow end to end.
func TestReviewsRoundTrip(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	itemID := itemIDFromPath(t, createItem(t, h, "Dune"))
	itemPath := "/catalog/" + itemID

	if resp := h.do(t, http.MethodPost, itemPath+"/rating", url.Values{"score": {"4"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST rating = %d, want 303", resp.StatusCode)
	}
	if resp := h.do(t, http.MethodPost, itemPath+"/note", url.Values{"body": {"Nos encantó."}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST note = %d, want 303", resp.StatusCode)
	}

	body := readBody(t, h.do(t, http.MethodGet, itemPath, nil))
	if !strings.Contains(body, `value="4" checked`) {
		t.Fatalf("own rating not rendered as checked: %s", body)
	}
	if !strings.Contains(body, "Nos encantó.") {
		t.Fatalf("shared note not rendered: %s", body)
	}
	if !strings.Contains(body, "Sin puntaje todavía") {
		t.Fatalf("partner rating placeholder missing: %s", body)
	}

	// The partner sees the same shared note and can add their own score.
	partner := loginAs(t, h, "Linus", "5678")
	if resp := doAs(t, partner, h, http.MethodPost, itemPath+"/rating", url.Values{"score": {"5"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("partner rating = %d, want 303", resp.StatusCode)
	}
	body = readBody(t, doAs(t, partner, h, http.MethodGet, itemPath, nil))
	if !strings.Contains(body, "Nos encantó.") || !strings.Contains(body, `value="5" checked`) {
		t.Fatalf("partner view missing review state: %s", body)
	}
}

// TestReviewsRespectItemPrivacy proves a partner can never rate a personal item.
func TestReviewsRespectItemPrivacy(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	itemID := itemIDFromPath(t, createItem(t, h, "Dune"))
	if resp := h.do(t, http.MethodPost, "/catalog/"+itemID+"/edit",
		url.Values{"title": {"Dune"}, "kind": {"movie"}, "personal": {"1"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("edit to personal = %d, want 303", resp.StatusCode)
	}

	partner := loginAs(t, h, "Linus", "5678")
	resp := doAs(t, partner, h, http.MethodPost, "/catalog/"+itemID+"/rating", url.Values{"score": {"3"}})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("partner rating on personal item = %d, want 404", resp.StatusCode)
	}
}

// TestRuletaPicksPendingOnly proves completed titles leave the spin pool and
// the chooser renders both categories.
func TestRuletaPicksPendingOnly(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	createItem(t, h, "Dune")
	createItem(t, h, "Alien")
	doneID := itemIDFromPath(t, createItem(t, h, "Old Joy"))
	if resp := h.do(t, http.MethodPost, "/progress/"+doneID+"/dates",
		url.Values{"done": {"on"}, "start": {"2026-08-14"}, "end": {"2026-08-20"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("complete item = %d, want 303", resp.StatusCode)
	}

	body := readBody(t, h.do(t, http.MethodGet, "/ruleta", nil))
	if !strings.Contains(body, "¿Qué vemos hoy?") || !strings.Contains(body, "Girar series") || !strings.Contains(body, "Girar películas") {
		t.Fatalf("chooser missing: %s", body)
	}

	body = readBody(t, h.do(t, http.MethodGet, "/ruleta?kind=movie", nil))
	if !strings.Contains(body, "Girar de nuevo") {
		t.Fatalf("movie spin missing: %s", body)
	}
	if strings.Contains(body, "Old Joy") {
		t.Fatalf("ruleta offered a completed title: %s", body)
	}
	if !strings.Contains(body, "Dune") && !strings.Contains(body, "Alien") {
		t.Fatalf("ruleta pick not rendered: %s", body)
	}
}

// TestBitacoraGroupsCompletedByMonth proves completed titles are grouped by
// their completion month and pending ones never appear.
func TestBitacoraGroupsCompletedByMonth(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	createItem(t, h, "Alien") // never completed: must not leak in
	itemID := itemIDFromPath(t, createItem(t, h, "Dune"))
	if resp := h.do(t, http.MethodPost, "/progress/"+itemID+"/dates",
		url.Values{"done": {"on"}, "start": {"2026-08-14"}, "end": {"2026-08-20"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("complete = %d, want 303", resp.StatusCode)
	}

	body := readBody(t, h.do(t, http.MethodGet, "/bitacora", nil))
	if !strings.Contains(body, "Agosto 2026") || !strings.Contains(body, "Dune") || !strings.Contains(body, "20 ago") {
		t.Fatalf("bitacora missing grouped entry: %s", body)
	}
	if strings.Contains(body, "Alien") {
		t.Fatalf("bitacora leaked a pending title: %s", body)
	}
}

// TestBackupDownload proves the UI backup streams a real SQLite snapshot.
func TestBackupDownload(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	resp := h.do(t, http.MethodPost, "/backup", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /backup = %d, want 200", resp.StatusCode)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, `attachment; filename="tracker-`) {
		t.Fatalf("Content-Disposition = %q, want attachment", cd)
	}
	body := readBody(t, resp)
	if !strings.HasPrefix(body, "SQLite format 3") {
		t.Fatalf("backup payload is not a SQLite database: %q", body[:min(20, len(body))])
	}
}

// TestCoverSearchFlow proves search, the same-origin candidate proxy and the
// adoption path all work through the real handlers with a stubbed network.
func TestCoverSearchFlow(t *testing.T) {
	h := newHarness(t)
	signIn(t, h)

	itemID := itemIDFromPath(t, createItem(t, h, "Dune"))
	itemPath := "/catalog/" + itemID

	body := readBody(t, h.do(t, http.MethodGet, itemPath+"/cover/search?q=Dune", nil))
	if !strings.Contains(body, "Stub · Dune") || !strings.Contains(body, "Usar esta") {
		t.Fatalf("cover search results missing: %s", body)
	}

	proxy := readBody(t, h.do(t, http.MethodGet,
		itemPath+"/cover/candidate?u="+url.QueryEscape("https://covers.openlibrary.org/b/id/1-M.jpg"), nil))
	if len(proxy) != len(tinyPNG) {
		t.Fatalf("candidate proxy returned %d bytes, want %d", len(proxy), len(tinyPNG))
	}

	if resp := h.do(t, http.MethodPost, itemPath+"/cover/use",
		url.Values{"u": {"https://covers.openlibrary.org/b/id/1-L.jpg"}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("cover use = %d, want 303", resp.StatusCode)
	}
	body = readBody(t, h.do(t, http.MethodGet, itemPath, nil))
	if !strings.Contains(body, "/uploads/") || !strings.Contains(body, "Buscar portada") {
		t.Fatalf("adopted cover not rendered: %s", body)
	}
}
