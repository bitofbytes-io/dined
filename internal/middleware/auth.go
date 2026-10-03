package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bitofbytes-io/dined/internal/auth"
)

const CookieName = "dined_session"

type userContextKey struct{}

// CurrentUser returns the signed-in user for r, or nil. Auth stores the user
// in the request context on protected routes, so this only reads the session
// store for public pages and routes outside Auth.
func CurrentUser(r *http.Request, authService *auth.Service) *auth.User {
	if user, ok := r.Context().Value(userContextKey{}).(*auth.User); ok {
		return user
	}
	cookie, err := r.Cookie(CookieName)
	if err != nil || cookie.Value == "" || authService == nil {
		return nil
	}
	user, err := authService.ValidateSession(r.Context(), cookie.Value)
	if err != nil {
		slog.Error("validate session", "error", err)
		return nil
	}
	return user
}

func Auth(authService *auth.Service, secureCookies bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicReadRequest(r) {
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie(CookieName)
			if err != nil || cookie.Value == "" {
				if err == nil {
					clearCookie(w, secureCookies)
				}
				slog.Info("browser authentication required", "path", r.URL.Path)
				if shouldReturnUnauthorized(r) {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				redirectToLogin(w, r)
				return
			}

			user, err := authService.ValidateSession(r.Context(), cookie.Value)
			if err != nil || user == nil {
				clearCookie(w, secureCookies)
				slog.Info("browser authentication failed", "reason", "invalid_session", "path", r.URL.Path)
				if shouldReturnUnauthorized(r) {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				redirectToLogin(w, r)
				return
			}

			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userContextKey{}, user)))
		})
	}
}

func SetSessionCookie(w http.ResponseWriter, token string, secure bool, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(ttl.Seconds()),
	})
}

func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	clearCookie(w, secure)
}

func isPublicReadRequest(r *http.Request) bool {
	if !isSafeMethod(r.Method) {
		return false
	}
	path := r.URL.Path
	return path == "/" ||
		path == "/dines" ||
		path == "/trophy-case" ||
		path == "/trophy-case/map.png" ||
		path == "/health" ||
		path == "/site.webmanifest" ||
		path == "/favicon.ico" ||
		// Dine photos appear on the public home, dines, and restaurant pages.
		strings.HasPrefix(path, "/photos/") ||
		isRestaurantPagePath(path) ||
		strings.HasPrefix(path, "/static/")
}

// isRestaurantPagePath matches only the read-only /restaurants/{id} page, so
// sub-pages such as /restaurants/{id}/edit still require a session.
func isRestaurantPagePath(path string) bool {
	id, ok := strings.CutPrefix(path, "/restaurants/")
	return ok && id != "" && !strings.Contains(id, "/")
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func shouldReturnUnauthorized(r *http.Request) bool {
	return strings.HasPrefix(r.URL.Path, "/api/") || !isSafeMethod(r.Method)
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) {
	target := "/login"
	if r.URL.String() != "/" {
		target += "?redirect=" + url.QueryEscape(r.URL.String())
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func clearCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
