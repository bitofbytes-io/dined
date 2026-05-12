package handler

import (
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/bitofbytes-io/dined/internal/ui"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	cfg    *config.Config
	store  repository.DinerStore
	places *places.Client
}

func New(cfg *config.Config, store repository.DinerStore, placesClient *places.Client) *Handler {
	return &Handler{cfg: cfg, store: store, places: placesClient}
}

func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	visits, err := h.store.Visits(r.Context(), 6)
	if err != nil {
		h.error(w, "home visits", err)
		return
	}
	h.render(w, "home", r, ui.PageData{Visits: visits})
}

func (h *Handler) Dines(w http.ResponseWriter, r *http.Request) {
	visits, err := h.store.Visits(r.Context(), 0)
	if err != nil {
		h.error(w, "all visits", err)
		return
	}
	h.render(w, "dines", r, ui.PageData{Title: "All Dines", Visits: visits})
}

func (h *Handler) Restaurant(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid restaurant ID", http.StatusBadRequest)
		return
	}
	restaurant, err := h.store.Restaurant(r.Context(), id)
	if err != nil {
		h.error(w, "restaurant", err)
		return
	}
	if restaurant == nil {
		http.NotFound(w, r)
		return
	}
	visits, err := h.store.RestaurantVisits(r.Context(), id)
	if err != nil {
		h.error(w, "restaurant visits", err)
		return
	}
	h.render(w, "restaurant", r, ui.PageData{Title: restaurant.Name, Restaurant: restaurant, Visits: visits})
}

func (h *Handler) Trophy(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.Stats(r.Context())
	if err != nil {
		h.error(w, "stats", err)
		return
	}
	h.render(w, "trophy", r, ui.PageData{Title: "Trophy Case", Stats: stats})
}

func (h *Handler) LogPage(w http.ResponseWriter, r *http.Request) {
	data, err := h.logData(r)
	if err != nil {
		h.error(w, "log page", err)
		return
	}
	h.render(w, "log", r, data)
}

func (h *Handler) CreateVisit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	input, err := h.visitInput(r)
	if err != nil {
		h.renderLogError(w, r, err.Error())
		return
	}
	visitID, err := h.store.CreateVisit(r.Context(), input)
	if err != nil {
		h.renderLogError(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/dines#"+visitID.String(), http.StatusSeeOther)
}

func (h *Handler) DeleteVisit(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid visit ID", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteVisit(r.Context(), id); err != nil {
		h.error(w, "delete visit", err)
		return
	}
	http.Redirect(w, r, "/dines", http.StatusSeeOther)
}

func (h *Handler) ToggleChain(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid restaurant ID", http.StatusBadRequest)
		return
	}
	isChain := r.FormValue("is_chain") == "true"
	if err := h.store.ToggleChain(r.Context(), id, isChain); err != nil {
		h.error(w, "toggle chain", err)
		return
	}
	http.Redirect(w, r, "/restaurants/"+id.String(), http.StatusSeeOther)
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	restaurants, err := h.store.Restaurants(r.Context(), q)
	if err != nil {
		h.error(w, "search restaurants", err)
		return
	}
	results, err := h.searchResults(r, restaurants)
	if err != nil {
		h.error(w, "search visit history", err)
		return
	}
	var found []places.Place
	if q != "" && h.places.Configured() {
		found, err = h.places.TextSearch(r.Context(), q)
		if err != nil {
			slog.Warn("places search failed", "error", err)
		}
	}
	h.render(w, "search", r, ui.PageData{Title: "Search", Query: q, SearchResults: results, Places: found})
}

func (h *Handler) Nearby(w http.ResponseWriter, r *http.Request) {
	var found []places.Place
	lat, latErr := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, lngErr := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	if latErr == nil && lngErr == nil && h.places.Configured() {
		places, err := h.places.Nearby(r.Context(), lat, lng)
		if err != nil {
			slog.Warn("nearby places failed", "error", err)
		} else {
			found = places
		}
	}
	h.render(w, "nearby", r, ui.PageData{Title: "Nearby", Places: found})
}

func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "login", r, ui.PageData{Title: "Login", Query: r.URL.Query().Get("redirect")})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	if r.FormValue("token") != h.cfg.APIToken {
		h.render(w, "login", r, ui.PageData{Title: "Login", Query: r.FormValue("redirect"), Error: "Invalid token"})
		return
	}
	middleware.SetSessionCookie(w, h.cfg.APIToken, h.cfg.SecureCookies)
	redirect := r.FormValue("redirect")
	if redirect == "" {
		redirect = "/"
	}
	http.Redirect(w, r, redirect, http.StatusSeeOther)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	middleware.ClearSessionCookie(w, h.cfg.SecureCookies)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) logData(r *http.Request) (ui.PageData, error) {
	people, err := h.store.People(r.Context())
	if err != nil {
		return ui.PageData{}, err
	}
	tags, err := h.store.Tags(r.Context())
	if err != nil {
		return ui.PageData{}, err
	}
	restaurants, err := h.store.Restaurants(r.Context(), "")
	if err != nil {
		return ui.PageData{}, err
	}
	prefillPrice, _ := strconv.Atoi(r.URL.Query().Get("price_level"))
	return ui.PageData{
		Title:               "Log a Dine",
		People:              people,
		Tags:                tags,
		Restaurants:         restaurants,
		PrefillName:         r.URL.Query().Get("restaurant_name"),
		PrefillAddress:      r.URL.Query().Get("address"),
		PrefillPlaceID:      r.URL.Query().Get("google_place_id"),
		PrefillCategory:     r.URL.Query().Get("category"),
		PrefillPriceLevel:   prefillPrice,
		PrefillRestaurantID: r.URL.Query().Get("restaurant_id"),
	}, nil
}

func (h *Handler) visitInput(r *http.Request) (model.VisitInput, error) {
	visitedAt, err := time.ParseInLocation("2006-01-02T15:04", r.FormValue("visited_at"), time.Local)
	if err != nil {
		return model.VisitInput{}, err
	}
	pickerID, err := uuid.Parse(r.FormValue("picker_id"))
	if err != nil {
		return model.VisitInput{}, err
	}
	priceLevel, _ := strconv.Atoi(r.FormValue("price_level"))
	input := model.VisitInput{
		RestaurantName: r.FormValue("restaurant_name"),
		Address:        r.FormValue("address"),
		GooglePlaceID:  r.FormValue("google_place_id"),
		Category:       r.FormValue("category"),
		IsChain:        r.FormValue("is_chain") == "true",
		VisitedAt:      visitedAt,
		PickerID:       pickerID,
		PriceLevel:     priceLevel,
		Notes:          r.FormValue("notes"),
		NewTag:         r.FormValue("new_tag"),
		Ratings:        map[uuid.UUID]float64{},
	}
	if restaurantID := r.FormValue("restaurant_id"); restaurantID != "" {
		id, err := uuid.Parse(restaurantID)
		if err != nil {
			return model.VisitInput{}, err
		}
		input.RestaurantID = &id
	}
	for key, values := range r.PostForm {
		if len(values) == 0 || len(key) < len("rating_") || key[:len("rating_")] != "rating_" || values[0] == "" {
			continue
		}
		personID, err := uuid.Parse(key[len("rating_"):])
		if err != nil {
			return model.VisitInput{}, err
		}
		score, err := strconv.ParseFloat(values[0], 64)
		if err != nil {
			return model.VisitInput{}, err
		}
		input.Ratings[personID] = score
	}
	for _, value := range r.PostForm["tag_id"] {
		tagID, err := uuid.Parse(value)
		if err != nil {
			return model.VisitInput{}, err
		}
		input.TagIDs = append(input.TagIDs, tagID)
	}
	return input, nil
}

func (h *Handler) renderLogError(w http.ResponseWriter, r *http.Request, message string) {
	data, err := h.logData(r)
	if err != nil {
		h.error(w, "log error data", err)
		return
	}
	data.Error = message
	h.render(w, "log", r, data)
}

func (h *Handler) render(w http.ResponseWriter, name string, r *http.Request, data ui.PageData) {
	data.Authenticated = middleware.IsAuthenticated(r, h.cfg.APIToken)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ui.Render(w, name, data); err != nil {
		slog.Error("render page", "page", name, "error", err)
	}
}

func (h *Handler) searchResults(r *http.Request, restaurants []model.Restaurant) ([]ui.RestaurantResult, error) {
	results := make([]ui.RestaurantResult, 0, len(restaurants))
	for _, restaurant := range restaurants {
		visits, err := h.store.RestaurantVisits(r.Context(), restaurant.ID)
		if err != nil {
			return nil, err
		}
		results = append(results, restaurantResult(restaurant, visits))
	}
	return results, nil
}

func restaurantResult(restaurant model.Restaurant, visits []model.Visit) ui.RestaurantResult {
	result := ui.RestaurantResult{Restaurant: restaurant}
	tagNames := map[string]model.Tag{}
	var ratingSum float64
	var ratingCount int
	for _, visit := range visits {
		result.VisitCount++
		if result.LatestVisit == nil || visit.VisitedAt.After(result.LatestVisit.VisitedAt) {
			visitCopy := visit
			result.LatestVisit = &visitCopy
		}
		for _, rating := range visit.Ratings {
			ratingSum += rating.Score
			ratingCount++
		}
		for _, tag := range visit.Tags {
			tagNames[strings.ToLower(tag.Name)] = tag
		}
	}
	if ratingCount > 0 {
		result.AverageRating = ratingSum / float64(ratingCount)
	}
	for _, tag := range tagNames {
		result.Tags = append(result.Tags, tag)
	}
	sort.Slice(result.Tags, func(i, j int) bool { return result.Tags[i].Name < result.Tags[j].Name })
	return result
}

func (h *Handler) error(w http.ResponseWriter, msg string, err error) {
	slog.Error(msg, "error", err)
	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
}
