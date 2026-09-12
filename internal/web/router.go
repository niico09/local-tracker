// Package web owns HTTP concerns: routing, middleware, rendering, and errors.
package web

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"time"
)

// RouterDeps carries the collaborators the HTTP layer needs. Health is a
// function rather than a storage type so web keeps no dependency on storage.
type RouterDeps struct {
	Health     func(ctx context.Context) (journalMode string, foreignKeys int, err error)
	Assets     fs.FS
	Auth       Auth
	Catalog    Catalog
	Goals      Goals
	Membership Membership
	Progress   Progress
	Covers     CoverFiles
	UploadMax  int64
}

// healthResponse is the JSON shape returned by GET /healthz.
type healthResponse struct {
	Status      string `json:"status"`
	JournalMode string `json:"journal_mode"`
	ForeignKeys int    `json:"foreign_keys"`
}

// NewRouter builds the application router using ServeMux method and wildcard
// patterns (Go 1.22+). S0 registers only the health probe and static assets;
// business routes arrive in later slices.
func NewRouter(deps RouterDeps) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", handleHealth(deps))

	if deps.Assets != nil {
		if static, err := fs.Sub(deps.Assets, "static"); err == nil {
			mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
		}
	}
	return mux
}

// NewServer builds the full HTTP handler: the base router plus the S1 auth
// routes, wrapped in the mandated middleware chain.
func NewServer(deps RouterDeps) (http.Handler, error) {
	mux := NewRouter(deps)
	tmpls, err := parseTemplates(deps.Assets)
	if err != nil {
		return nil, err
	}
	limiter := newRateLimiter(5, 15*time.Minute)

	mux.HandleFunc("GET /setup", handleSetupGet(deps.Auth, tmpls))
	mux.HandleFunc("POST /setup", handleSetupPost(deps.Auth, tmpls))
	mux.HandleFunc("GET /login", handleLoginGet(tmpls))
	mux.HandleFunc("POST /login", handleLoginPost(deps.Auth, limiter, tmpls))
	mux.HandleFunc("POST /logout", handleLogout(deps.Auth))
	mux.HandleFunc("GET /", handleDashboard(tmpls))

	// Catalog routes. GET /catalog/new is a literal pattern and therefore wins
	// over GET /catalog/{id} by ServeMux specificity regardless of order.
	if deps.Catalog != nil {
		mux.HandleFunc("GET /catalog", handleCatalogList(deps.Catalog, tmpls))
		mux.HandleFunc("POST /catalog", handleCatalogCreate(deps.Catalog, tmpls))
		mux.HandleFunc("GET /catalog/new", handleCatalogNew(tmpls))
		mux.HandleFunc("POST /catalog/new", handleCatalogCreate(deps.Catalog, tmpls))
		mux.HandleFunc("GET /catalog/{id}", handleCatalogDetail(deps.Catalog, tmpls))
		mux.HandleFunc("GET /catalog/{id}/edit", handleCatalogEdit(deps.Catalog, tmpls))
		mux.HandleFunc("POST /catalog/{id}/edit", handleCatalogUpdate(deps.Catalog, tmpls))
		mux.HandleFunc("POST /catalog/{id}/delete", handleCatalogDelete(deps.Catalog))
		mux.HandleFunc("POST /catalog/{id}/cover", handleCatalogCover(deps.Catalog, deps.UploadMax))
	}
	if deps.Covers != nil {
		mux.HandleFunc("GET /uploads/{name}", handleUploads(deps.Covers))
	}

	// Goal routes. GET /goals/new is a literal pattern and therefore wins over
	// GET /goals/{id} by ServeMux specificity regardless of order.
	if deps.Goals != nil {
		mux.HandleFunc("GET /goals", handleGoalList(deps.Goals, tmpls))
		mux.HandleFunc("POST /goals", handleGoalCreate(deps.Goals, tmpls))
		mux.HandleFunc("GET /goals/new", handleGoalNew(tmpls))
		mux.HandleFunc("POST /goals/new", handleGoalCreate(deps.Goals, tmpls))
		mux.HandleFunc("GET /goals/{id}", handleGoalDetail(deps.Goals, deps.Membership, deps.Catalog, tmpls))
		mux.HandleFunc("GET /goals/{id}/edit", handleGoalEdit(deps.Goals, tmpls))
		mux.HandleFunc("POST /goals/{id}/edit", handleGoalUpdate(deps.Goals, tmpls))
		mux.HandleFunc("POST /goals/{id}/delete", handleGoalDelete(deps.Goals))
		mux.HandleFunc("POST /goals/{id}/visibility", handleGoalVisibility(deps.Goals))
		if deps.Membership != nil {
			mux.HandleFunc("POST /goals/{id}/items", handleMembershipAdd(deps.Membership))
			mux.HandleFunc("POST /goals/{id}/items/{itemID}/remove", handleMembershipRemove(deps.Membership))
		}
	}

	// Progress routes: the tilt mutations degrade to 303 without HTMX, and the
	// two GET endpoints serve a fragment to HTMX or the full page otherwise.
	if deps.Progress != nil {
		mux.HandleFunc("POST /progress/{itemID}/toggle", handleProgressToggle(deps.Progress, tmpls))
		mux.HandleFunc("POST /progress/{itemID}/dates", handleProgressDates(deps.Progress, tmpls))
		mux.HandleFunc("GET /goals/{id}/progress", handleGoalProgress(deps.Progress, tmpls))
		mux.HandleFunc("GET /catalog/{id}/tilt", handleCatalogTilt(deps.Progress, tmpls))
	}

	return Chain(mux,
		Recover,
		Logging,
		SecurityHeaders,
		SameOrigin,
		func(next http.Handler) http.Handler { return SetupGate(deps.Auth, next) },
		func(next http.Handler) http.Handler { return Authenticate(deps.Auth, next) },
		RequireAuth,
	), nil
}

// handleHealth reports the live SQLite pragma state; it returns 200 only when
// journal_mode and foreign_keys are readable.
func handleHealth(deps RouterDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := healthResponse{Status: "ok"}
		status := http.StatusOK

		if deps.Health == nil {
			resp.Status = "unavailable"
			status = http.StatusServiceUnavailable
		} else if journal, fk, err := deps.Health(r.Context()); err != nil {
			resp.Status = "unavailable"
			status = http.StatusServiceUnavailable
		} else {
			resp.JournalMode = journal
			resp.ForeignKeys = fk
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(resp)
	}
}
