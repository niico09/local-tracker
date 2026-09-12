// Package web owns HTTP concerns: routing, middleware, rendering, and errors.
package web

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
)

// RouterDeps carries the collaborators the HTTP layer needs. Health is a
// function rather than a storage type so web keeps no dependency on storage.
type RouterDeps struct {
	Health func(ctx context.Context) (journalMode string, foreignKeys int, err error)
	Assets fs.FS
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
