package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/bitofbytes-io/dined/internal/auth"
	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/handler"
	"github.com/bitofbytes-io/dined/internal/metrics"
	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

// readyTimeout bounds the database ping behind /ready.
const readyTimeout = 2 * time.Second

type Server struct {
	cfg         *config.Config
	store       repository.DinerStore
	places      *places.Client
	authService *auth.Service
	googleAuth  *auth.GoogleAuthenticator
}

func New(cfg *config.Config, store repository.DinerStore, placesClient *places.Client, authService *auth.Service, googleAuth *auth.GoogleAuthenticator) *Server {
	return &Server{cfg: cfg, store: store, places: placesClient, authService: authService, googleAuth: googleAuth}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(middleware.RealIP(s.cfg.TrustedProxies))
	r.Use(middleware.Logger)
	r.Use(metrics.Middleware)
	r.Use(chimw.Recoverer)
	r.Use(middleware.SameOrigin)

	static := http.FileServer(http.Dir("static"))
	r.Handle("/static/*", http.StripPrefix("/static/", static))
	r.Get("/favicon.ico", serveFile("static/favicon.ico"))
	r.Get("/favicon-16x16.png", serveFile("static/favicon-16x16.png"))
	r.Get("/favicon-32x32.png", serveFile("static/favicon-32x32.png"))
	r.Get("/apple-touch-icon.png", serveFile("static/apple-touch-icon.png"))
	r.Get("/android-chrome-192x192.png", serveFile("static/android-chrome-192x192.png"))
	r.Get("/android-chrome-512x512.png", serveFile("static/android-chrome-512x512.png"))
	r.Get("/site.webmanifest", serveFile("static/site.webmanifest"))
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/ready", s.ready)

	h := handler.New(s.cfg, s.store, s.places, s.authService, s.googleAuth)
	r.Get("/login", h.LoginPage)
	r.Post("/login", h.Login)
	r.Get("/api/auth/google", h.StartGoogleLogin)
	r.Get("/api/auth/google/callback", h.GoogleCallback)
	r.Post("/logout", h.Logout)

	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(s.authService, s.cfg.SecureCookies))
		r.Get("/", h.Home)
		r.Get("/dines", h.Dines)
		r.Get("/photos/{id}", h.Photo)
		r.Get("/restaurants/{id}", h.Restaurant)
		r.Get("/restaurants/{id}/edit", h.EditRestaurantPage)
		r.Post("/restaurants/{id}", h.UpdateRestaurant)
		r.Get("/trophy-case", h.Trophy)
		r.Get("/trophy-case/map.png", h.TrophyMap)
		r.Get("/want-to-try", h.WantToTry)
		r.Get("/want-to-try/new", h.NewWantToTryPage)
		r.Post("/want-to-try", h.SaveWantToTry)
		r.Post("/want-to-try/{id}/delete", h.DeleteWantToTry)

		r.Get("/log", h.LogPage)
		r.Post("/visits", h.CreateVisit)
		r.Get("/visits/{id}/edit", h.EditVisitPage)
		r.Post("/visits/{id}", h.UpdateVisit)
		r.Post("/visits/{id}/delete", h.DeleteVisit)
		r.Post("/restaurants/{id}/delete", h.DeleteRestaurant)
		r.Post("/restaurants/{id}/chain", h.ToggleChain)
		r.Post("/restaurants/{id}/google-refresh", h.RefreshRestaurantGoogle)
		r.Get("/search", h.Search)
		r.Get("/nearby", h.Nearby)
	})

	return r
}

// ready reports whether dined's dependencies are reachable, unlike /health,
// which only reports that the process is up. The response never includes the
// error, which can name the database host or user.
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readyTimeout)
	defer cancel()

	status, db, code := "ok", "ok", http.StatusOK
	if err := s.store.Ping(ctx); err != nil {
		slog.Error("readiness check failed", "dependency", "db", "error", err)
		status, db, code = "degraded", "error", http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
		DB     string `json:"db"`
	}{status, db})
}

func serveFile(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, path)
	}
}
