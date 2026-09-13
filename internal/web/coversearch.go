package web

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"local-tracker/internal/coverfetch"
	"local-tracker/internal/domain"
)

// CoverFinder searches and fetches candidate covers for the automatic cover
// picker. The feature is optional: its routes only exist when a finder is
// wired, so a setup without outbound access keeps the manual upload flow.
type CoverFinder interface {
	Search(ctx context.Context, query string) ([]coverfetch.Candidate, error)
	Fetch(ctx context.Context, url string) ([]byte, string, error)
}

// handleCoverSearch renders the candidate list for one item.
func handleCoverSearch(cat Catalog, finder CoverFinder, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := itemID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		item, err := cat.Get(r.Context(), actorOf(r), id)
		if err != nil {
			fail(w, r, err)
			return
		}
		v := toView(item)
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if query == "" {
			query = item.Title
		}
		data := pageData{Title: "Buscar portada", Item: &v, Query: query, CanSearchCover: true, Searched: true}
		candidates, err := finder.Search(r.Context(), query)
		if err != nil {
			data.Error = "No se pudieron encontrar portadas para ese texto. Prueba con otro título o sube una imagen a mano."
		}
		data.Candidates = candidates
		render(w, tmpls, "cover_search", http.StatusOK, data)
	}
}

// handleCoverCandidate proxies one candidate image so the browser only ever
// loads same-origin bytes (the server CSP forbids anything else).
func handleCoverCandidate(cat Catalog, finder CoverFinder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := itemID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		if _, err := cat.Get(r.Context(), actorOf(r), id); err != nil {
			fail(w, r, err)
			return
		}
		data, contentType, err := finder.Fetch(r.Context(), r.URL.Query().Get("u"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "private, max-age=300")
		_, _ = w.Write(data)
	}
}

// handleCoverUse downloads the chosen candidate and stores it as the item's
// cover, reusing the exact validation path as a manual upload.
func handleCoverUse(cat Catalog, finder CoverFinder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := itemID(r)
		if !ok {
			fail(w, r, domain.ErrNotFound)
			return
		}
		data, _, err := finder.Fetch(r.Context(), strings.TrimSpace(r.PostFormValue("u")))
		if err != nil {
			fail(w, r, fmt.Errorf("%w: no se pudo descargar la portada elegida", domain.ErrValidation))
			return
		}
		if err := cat.SetCover(r.Context(), actorOf(r), id, data); err != nil {
			fail(w, r, err)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/catalog/%d", id), http.StatusSeeOther)
	}
}
