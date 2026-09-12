package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouterHealthzAndRegistration(t *testing.T) {
	deps := RouterDeps{
		Health: func(context.Context) (string, int, error) { return "wal", 1, nil },
	}

	var mux *http.ServeMux
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("NewRouter panicked: %v", r)
			}
		}()
		mux = NewRouter(deps)
	}()

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body struct {
		Status      string `json:"status"`
		JournalMode string `json:"journal_mode"`
		ForeignKeys int    `json:"foreign_keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode /healthz: %v", err)
	}
	if body.Status != "ok" || body.JournalMode != "wal" || body.ForeignKeys != 1 {
		t.Errorf("healthz body = %+v, want ok/wal/1", body)
	}
}
