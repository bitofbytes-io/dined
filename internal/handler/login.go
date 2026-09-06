package handler

import (
	"log/slog"
	"net/http"

	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/ui"
)

func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	data := ui.PageData{Title: "Login", Query: r.URL.Query().Get("redirect")}
	if message := r.URL.Query().Get("message"); message != "" {
		data.Error = message
	}
	h.render(w, "login", r, data)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	slog.Info("login requested", "provider", "google")
	http.Redirect(w, r, googleLoginPath(r.FormValue("redirect")), http.StatusSeeOther)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	hadSession := false
	if cookie, err := r.Cookie(middleware.CookieName); err == nil && cookie.Value != "" && h.authService != nil {
		hadSession = true
		if err := h.authService.DeleteSession(r.Context(), cookie.Value); err != nil {
			slog.Error("delete session", "error", err)
		}
	}
	middleware.ClearSessionCookie(w, h.cfg.SecureCookies)
	slog.Info("logout", "had_session", hadSession)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
