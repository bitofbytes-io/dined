package server

import (
	"net/http"

	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/handler"
	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type Server struct {
	cfg    *config.Config
	store  repository.DinerStore
	places *places.Client
}

func New(cfg *config.Config, store repository.DinerStore, placesClient *places.Client) *Server {
	return &Server{cfg: cfg, store: store, places: placesClient}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)

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

	h := handler.New(s.cfg, s.store, s.places)
	r.Get("/login", h.LoginPage)
	r.Post("/login", h.Login)

	r.Group(func(r chi.Router) {
		r.Use(middleware.Auth(s.cfg.APIToken, s.cfg.SecureCookies))
		r.Get("/", h.Home)
		r.Get("/dines", h.Dines)
		r.Get("/restaurants/{id}", h.Restaurant)
		r.Get("/restaurants/{id}/edit", h.EditRestaurantPage)
		r.Post("/restaurants/{id}", h.UpdateRestaurant)
		r.Get("/trophy-case", h.Trophy)

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
		r.Post("/logout", h.Logout)
	})

	return r
}

func serveFile(path string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, path)
	}
}
