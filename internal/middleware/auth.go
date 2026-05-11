package middleware

import (
	"crypto/subtle"
	"net/http"
	"net/url"
	"strings"
)

const CookieName = "dined_session"

func Auth(apiToken string, secureCookies bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isPublicReadRequest(r) {
				next.ServeHTTP(w, r)
				return
			}

			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				token, ok := strings.CutPrefix(authHeader, "Bearer ")
				if ok && constantTimeEqual(token, apiToken) {
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			cookie, err := r.Cookie(CookieName)
			if err != nil || !constantTimeEqual(cookie.Value, apiToken) {
				if err == nil {
					clearCookie(w, secureCookies)
				}
				if shouldReturnUnauthorized(r) {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				redirectToLogin(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func SetSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   60 * 60 * 24 * 365,
	})
}

func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	clearCookie(w, secure)
}

func IsAuthenticated(r *http.Request, apiToken string) bool {
	cookie, err := r.Cookie(CookieName)
	return err == nil && constantTimeEqual(cookie.Value, apiToken)
}

func isPublicReadRequest(r *http.Request) bool {
	if !isSafeMethod(r.Method) {
		return false
	}
	path := r.URL.Path
	return path == "/" ||
		path == "/dines" ||
		path == "/trophy-case" ||
		path == "/health" ||
		path == "/site.webmanifest" ||
		path == "/favicon.ico" ||
		strings.HasPrefix(path, "/restaurants/") ||
		strings.HasPrefix(path, "/static/")
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
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func constantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
