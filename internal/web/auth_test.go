package web_test

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"local-tracker/internal/domain"
	"local-tracker/internal/service"
	"local-tracker/internal/storage"
	"local-tracker/internal/ui"
	"local-tracker/internal/web"
)

type harness struct {
	srv        *httptest.Server
	repos      *storage.Repos
	client     *http.Client
	covers     *service.CoverStore
	uploadsDir string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()
	dataDir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dataDir, "tracker.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repos := storage.NewRepos(db)
	auth := service.NewAuth(repos.Users, repos.Sessions, time.Hour)
	uploadsDir := filepath.Join(dataDir, "uploads")
	covers := service.NewCoverStore(uploadsDir, 5<<20)
	catalog := service.NewCatalog(repos.Items, covers)
	goals := service.NewGoals(repos.Goals)
	members := service.NewMembership(repos.Membership)
	progress := service.NewProgress(repos.Progress)
	handler, err := web.NewServer(web.RouterDeps{
		Health:     db.Health,
		Assets:     ui.FS(),
		Auth:       auth,
		Catalog:    catalog,
		Goals:      goals,
		Membership: members,
		Progress:   progress,
		Covers:     covers,
		UploadMax:  5 << 20,
	})
	if err != nil {
		t.Fatalf("web.NewServer: %v", err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar: %v", err)
	}
	client := &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &harness{srv: srv, repos: repos, client: client, covers: covers, uploadsDir: uploadsDir}
}

func (h *harness) do(t *testing.T, method, path string, form url.Values) *http.Response {
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
	resp, err := h.client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func setupForm() url.Values {
	return url.Values{"name1": {"Ada"}, "pin1": {"1234"}, "name2": {"Linus"}, "pin2": {"5678"}}
}

func loginForm(name, pin string) url.Values {
	return url.Values{"name": {name}, "pin": {pin}}
}

func TestSetupGateRedirectsAndCloses(t *testing.T) {
	h := newHarness(t)

	// No users yet: every protected path is forced to /setup.
	resp := h.do(t, http.MethodGet, "/", nil)
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/setup" {
		t.Fatalf("GET / before setup = %d %q, want 302 /setup", resp.StatusCode, resp.Header.Get("Location"))
	}

	if resp := h.do(t, http.MethodPost, "/setup", setupForm()); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /setup = %d, want 303", resp.StatusCode)
	}

	// Users now exist: setup is closed and redirects to login.
	resp = h.do(t, http.MethodGet, "/setup", nil)
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/login" {
		t.Fatalf("GET /setup after setup = %d %q, want 302 /login", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestLoginCookieFlags(t *testing.T) {
	h := newHarness(t)
	h.do(t, http.MethodPost, "/setup", setupForm())

	resp := h.do(t, http.MethodPost, "/login", loginForm("Ada", "1234"))
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /login = %d, want 303", resp.StatusCode)
	}
	var session *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "lt_session" {
			session = c
		}
	}
	if session == nil {
		t.Fatal("login did not set lt_session")
	}
	if !session.HttpOnly {
		t.Error("session cookie must be HttpOnly")
	}
	if session.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", session.SameSite)
	}
	if session.Secure {
		t.Error("session cookie must not be Secure on plaintext LAN HTTP")
	}
}

func TestLoginRateLimit(t *testing.T) {
	h := newHarness(t)
	h.do(t, http.MethodPost, "/setup", setupForm())

	for i := 1; i <= 5; i++ {
		resp := h.do(t, http.MethodPost, "/login", loginForm("Ada", "0000"))
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("wrong PIN attempt %d = %d, want 401", i, resp.StatusCode)
		}
	}
	if resp := h.do(t, http.MethodPost, "/login", loginForm("Ada", "1234")); resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("6th attempt = %d, want 429", resp.StatusCode)
	}
}

func TestExpiredSessionIsAnonymous(t *testing.T) {
	h := newHarness(t)
	h.do(t, http.MethodPost, "/setup", setupForm())

	ctx := context.Background()
	past := time.Now().Add(-time.Hour)
	token := "expired-token"
	if err := h.repos.Sessions.Create(ctx, domain.SystemActor(), domain.Session{
		TokenHash: domain.HashToken(token), UserID: 1, CreatedAt: past, ExpiresAt: past,
	}); err != nil {
		t.Fatalf("seed expired session: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, h.srv.URL+"/", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: "lt_session", Value: token})
	resp, err := h.client.Do(req)
	if err != nil {
		t.Fatalf("GET / with expired cookie: %v", err)
	}
	if resp.StatusCode != http.StatusFound || resp.Header.Get("Location") != "/login" {
		t.Fatalf("expired session = %d %q, want 302 /login", resp.StatusCode, resp.Header.Get("Location"))
	}
	if _, err := h.repos.Sessions.FindByTokenHash(ctx, domain.SystemActor(), domain.HashToken(token)); err == nil {
		t.Error("expired session row was not revoked")
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	h := newHarness(t)
	h.do(t, http.MethodPost, "/setup", setupForm())

	resp := h.do(t, http.MethodPost, "/login", loginForm("Ada", "1234"))
	var token string
	for _, c := range resp.Cookies() {
		if c.Name == "lt_session" {
			token = c.Value
		}
	}
	if token == "" {
		t.Fatal("login did not return a session token")
	}
	if resp := h.do(t, http.MethodGet, "/", nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / while signed in = %d, want 200", resp.StatusCode)
	}

	if resp := h.do(t, http.MethodPost, "/logout", nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /logout = %d, want 303", resp.StatusCode)
	}
	ctx := context.Background()
	if _, err := h.repos.Sessions.FindByTokenHash(ctx, domain.SystemActor(), domain.HashToken(token)); err == nil {
		t.Error("session row still stored after logout")
	}
	if resp := h.do(t, http.MethodGet, "/", nil); resp.StatusCode != http.StatusFound {
		t.Fatalf("GET / after logout = %d, want 302", resp.StatusCode)
	}
}
