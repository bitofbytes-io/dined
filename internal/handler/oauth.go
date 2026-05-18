package handler

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bitofbytes-io/dined/internal/auth"
	"github.com/bitofbytes-io/dined/internal/middleware"
)

const (
	oauthStateCookieName = "dined_oauth_state"
	oauthStateCookieTTL  = 10 * time.Minute
)

type googleAuthenticator interface {
	AuthURL(state string) string
	Exchange(ctx context.Context, code string) (*auth.GoogleClaims, error)
	IsEmailAllowed(email string) bool
}

type oauthStatePayload struct {
	State    string `json:"s"`
	Redirect string `json:"r,omitempty"`
}

func (h *Handler) StartGoogleLogin(w http.ResponseWriter, r *http.Request) {
	state, err := auth.GenerateState()
	if err != nil {
		slog.Error("generate oauth state", "error", err)
		http.Error(w, "Unable to start login", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    state,
		Path:     "/api/auth",
		HttpOnly: true,
		Secure:   h.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(oauthStateCookieTTL.Seconds()),
	})

	payload := oauthStatePayload{State: state}
	if redirect := r.URL.Query().Get("redirect"); isSafeRedirectPath(redirect) {
		payload.Redirect = redirect
	}

	stateJSON, _ := json.Marshal(payload)
	encodedState := base64.RawURLEncoding.EncodeToString(stateJSON)
	http.Redirect(w, r, h.googleAuth.AuthURL(encodedState), http.StatusTemporaryRedirect)
}

func (h *Handler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie(oauthStateCookieName)
	if err != nil || stateCookie.Value == "" {
		h.redirectLoginError(w, r, "Session expired. Please try again.")
		return
	}

	redirect := "/"
	stateParam := r.URL.Query().Get("state")
	stateBytes, err := base64.RawURLEncoding.DecodeString(stateParam)
	if err != nil {
		h.redirectLoginError(w, r, "Invalid login state. Please try again.")
		return
	}

	var payload oauthStatePayload
	if err := json.Unmarshal(stateBytes, &payload); err != nil {
		h.redirectLoginError(w, r, "Invalid login state. Please try again.")
		return
	}
	if subtle.ConstantTimeCompare([]byte(payload.State), []byte(stateCookie.Value)) != 1 {
		h.redirectLoginError(w, r, "Invalid login state. Please try again.")
		return
	}
	if isSafeRedirectPath(payload.Redirect) {
		redirect = payload.Redirect
	}

	clearOAuthStateCookie(w, h.cfg.SecureCookies)

	if errParam := r.URL.Query().Get("error"); errParam != "" {
		message := r.URL.Query().Get("error_description")
		if message == "" {
			message = "Google login was not completed."
		}
		slog.Warn("oauth provider error", "error", errParam)
		h.redirectLoginError(w, r, message)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		h.redirectLoginError(w, r, "Google did not return an authorization code.")
		return
	}

	claims, err := h.googleAuth.Exchange(r.Context(), code)
	if err != nil {
		slog.Error("oauth exchange", "error", err)
		h.redirectLoginError(w, r, "Failed to complete Google login.")
		return
	}
	if !claims.EmailVerified {
		slog.Warn("oauth email not verified", "email", claims.Email)
		h.redirectLoginError(w, r, "Please verify your Google email address before logging in.")
		return
	}
	if !h.googleAuth.IsEmailAllowed(claims.Email) {
		slog.Warn("oauth email not allowed", "email", claims.Email)
		h.redirectLoginError(w, r, "That Google account is not allowed to access Dined.")
		return
	}

	user, err := h.authService.CreateOrUpdateUser(r.Context(), claims)
	if err != nil {
		slog.Error("oauth user", "error", err)
		h.redirectLoginError(w, r, "Failed to create your Dined session.")
		return
	}

	token, err := h.authService.CreateSession(r.Context(), user.ID, r.UserAgent(), clientIPFromRequest(r))
	if err != nil {
		slog.Error("oauth session", "error", err)
		h.redirectLoginError(w, r, "Failed to create your Dined session.")
		return
	}

	middleware.SetSessionCookie(w, token, h.cfg.SecureCookies, h.cfg.AuthSessionTTL)
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (h *Handler) redirectLoginError(w http.ResponseWriter, r *http.Request, message string) {
	target := "/login?message=" + url.QueryEscape(message)
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func googleLoginPath(redirect string) string {
	target := "/api/auth/google"
	if isSafeRedirectPath(redirect) {
		target += "?redirect=" + url.QueryEscape(redirect)
	}
	return target
}

func isSafeRedirectPath(path string) bool {
	if path == "" {
		return false
	}
	decoded, err := url.QueryUnescape(path)
	if err != nil {
		return false
	}
	if !strings.HasPrefix(decoded, "/") || strings.HasPrefix(decoded, "//") {
		return false
	}
	parsed, err := url.Parse(decoded)
	if err != nil {
		return false
	}
	return parsed.Scheme == "" && parsed.Host == ""
}

func clearOAuthStateCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName,
		Value:    "",
		Path:     "/api/auth",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func clientIPFromRequest(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
