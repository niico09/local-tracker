package web

import (
	"context"
	"html/template"
	"io/fs"
	"log"
	"net"
	"net/http"
	"time"

	"local-tracker/internal/domain"
	"local-tracker/internal/service"
)

const sessionCookie = "lt_session"

// Auth is the auth use-case surface the HTTP layer depends on.
type Auth interface {
	NeedsSetup(ctx context.Context) (bool, error)
	Setup(ctx context.Context, in service.SetupInput) error
	Login(ctx context.Context, name, pin string) (string, error)
	Authenticate(ctx context.Context, token string) (domain.Actor, error)
	Logout(ctx context.Context, a domain.Actor, token string) error
	SessionTTL() time.Duration
}

// pageData is the template payload shared by every page.
type pageData struct {
	Title        string
	Error        string
	UserID       domain.UserID
	Items        []itemView
	Item         *itemView
	Kinds        []domain.Kind
	Form         itemForm
	Goals        []goalView
	Goal         *goalView
	GoalForm     goalForm
	Visibilities []domain.Visibility
}

func handleSetupGet(auth Auth, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		needs, err := auth.NeedsSetup(r.Context())
		if err != nil {
			fail(w, r, err)
			return
		}
		if !needs {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		render(w, tmpls, "setup", http.StatusOK, pageData{Title: "Set up profiles"})
	}
}

func handleSetupPost(auth Auth, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		needs, err := auth.NeedsSetup(r.Context())
		if err != nil {
			fail(w, r, err)
			return
		}
		if !needs {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		in := service.SetupInput{
			Name1: r.FormValue("name1"), Pin1: r.FormValue("pin1"),
			Name2: r.FormValue("name2"), Pin2: r.FormValue("pin2"),
		}
		if err := auth.Setup(r.Context(), in); err != nil {
			fail(w, r, err)
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

func handleLoginGet(tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		render(w, tmpls, "login", http.StatusOK, pageData{Title: "Sign in"})
	}
}

func handleLoginPost(auth Auth, limiter *rateLimiter, tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name, pin := r.FormValue("name"), r.FormValue("pin")
		key := clientIP(r) + "|" + name
		if !limiter.Allow(key) {
			http.Error(w, "too many attempts", http.StatusTooManyRequests)
			return
		}
		token, err := auth.Login(r.Context(), name, pin)
		if err != nil {
			limiter.Fail(key)
			time.Sleep(25 * time.Millisecond) // constant cost per failure
			render(w, tmpls, "login", http.StatusUnauthorized, pageData{Title: "Sign in", Error: "Invalid name or PIN"})
			return
		}
		limiter.Reset(key)
		setSessionCookie(w, token, auth.SessionTTL())
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func handleLogout(auth Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := ActorFrom(r.Context())
		if cookie, err := r.Cookie(sessionCookie); err == nil {
			_ = auth.Logout(r.Context(), actor, cookie.Value)
		}
		clearSessionCookie(w)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

func handleDashboard(tmpls templates) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		actor, _ := ActorFrom(r.Context())
		render(w, tmpls, "dashboard", http.StatusOK, pageData{Title: "Dashboard", UserID: actor.ID()})
	}
}

// setSessionCookie writes the session cookie. It is HttpOnly and SameSite=Lax,
// and deliberately not Secure because the app runs on plaintext LAN HTTP.
func setSessionCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false,
		MaxAge:   int(ttl.Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   false,
		MaxAge:   -1,
	})
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// templates maps a page name to its parsed layout+content template.
type templates map[string]*template.Template

// parseTemplates parses base.html plus each page separately so every page can
// define its own "content" block without name collisions.
func parseTemplates(assets fs.FS) (templates, error) {
	pages := []string{"setup", "login", "dashboard", "catalog_list", "catalog_detail", "catalog_new", "catalog_edit",
		"goals_list", "goal_detail", "goal_new", "goal_edit"}
	out := make(templates, len(pages))
	for _, name := range pages {
		t, err := template.ParseFS(assets, "templates/base.html", "templates/pages/"+name+".html")
		if err != nil {
			return nil, err
		}
		out[name] = t
	}
	return out, nil
}

func render(w http.ResponseWriter, tmpls templates, name string, status int, data pageData) {
	t, ok := tmpls[name]
	if !ok {
		http.Error(w, "template missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}
