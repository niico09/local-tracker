package web

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"local-tracker/internal/domain"
)

type ctxKey int

const actorKey ctxKey = iota

// ActorFrom returns the authenticated actor stored by Authenticate.
func ActorFrom(ctx context.Context) (domain.Actor, bool) {
	a, ok := ctx.Value(actorKey).(domain.Actor)
	return a, ok
}

// Chain applies middleware so that the first argument is the outermost layer.
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// Recover turns a panic into a 500 instead of killing the process.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic: %v", rec)
				http.Error(w, "internal error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// Logging writes one line per request.
func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// SecurityHeaders sets the baseline response headers.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// SameOrigin rejects state-changing requests that explicitly declare a
// cross-site origin. SameSite=Lax on the cookie is the second layer.
func SameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
				http.Error(w, "cross-origin request rejected", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// SetupGate forces every request to /setup until the two profiles exist.
func SetupGate(auth Auth, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth == nil || isSetupPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		needs, err := auth.NeedsSetup(r.Context())
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if needs {
			redirect(w, r, "/setup")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Authenticate resolves the session cookie into an actor in the context.
func Authenticate(auth Auth, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth == nil {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(sessionCookie)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		actor, err := auth.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), actorKey, actor)))
	})
}

// RequireAuth sends anonymous requests to login.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := ActorFrom(r.Context()); ok {
			next.ServeHTTP(w, r)
			return
		}
		redirect(w, r, "/login")
	})
}

// redirect uses 401 + HX-Redirect for HTMX requests and 302 otherwise.
func redirect(w http.ResponseWriter, r *http.Request, location string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", location)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, location, http.StatusFound)
}

func isSetupPath(p string) bool {
	return p == "/setup" || p == "/healthz" || p == "/manifest.webmanifest" ||
		strings.HasPrefix(p, "/static/") || strings.HasPrefix(p, "/uploads/")
}

func isPublicPath(p string) bool {
	return p == "/healthz" || p == "/login" || p == "/setup" || p == "/manifest.webmanifest" ||
		strings.HasPrefix(p, "/static/") || strings.HasPrefix(p, "/uploads/")
}
