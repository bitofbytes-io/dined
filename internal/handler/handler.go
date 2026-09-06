package handler

import (
	"context"
	"log/slog"
	"net/http"
	"sync"

	"github.com/bitofbytes-io/dined/internal/auth"
	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/bitofbytes-io/dined/internal/ui"
)

type googlePlacesClient interface {
	Configured() bool
	TextSearch(context.Context, string) ([]places.Place, error)
	TextSearchNear(context.Context, string, float64, float64) ([]places.Place, error)
	Nearby(context.Context, float64, float64) ([]places.Place, error)
	Details(context.Context, string) (*places.Place, error)
	StaticMap(context.Context, places.StaticMapRequest) (*places.StaticMapImage, error)
}

type Handler struct {
	cfg         *config.Config
	store       repository.DinerStore
	places      googlePlacesClient
	authService *auth.Service
	googleAuth  googleAuthenticator
	mapCacheMu  sync.Mutex
	mapCache    trophyMapCacheEntry
}

func New(cfg *config.Config, store repository.DinerStore, placesClient googlePlacesClient, authService *auth.Service, googleAuth googleAuthenticator) *Handler {
	return &Handler{cfg: cfg, store: store, places: placesClient, authService: authService, googleAuth: googleAuth}
}

func (h *Handler) placesConfigured() bool {
	return h.places != nil && h.places.Configured()
}

func (h *Handler) render(w http.ResponseWriter, name string, r *http.Request, data ui.PageData) {
	data.Authenticated = middleware.IsAuthenticated(r, h.authService)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ui.Render(w, name, data); err != nil {
		slog.Error("render page", "page", name, "error", err)
	}
}

func (h *Handler) error(w http.ResponseWriter, msg string, err error) {
	slog.Error(msg, "error", err)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}
