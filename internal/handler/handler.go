package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/auth"
	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/placesync"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/bitofbytes-io/dined/internal/ui"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	cfg         *config.Config
	store       repository.DinerStore
	places      *places.Client
	authService *auth.Service
	googleAuth  googleAuthenticator
}

func New(cfg *config.Config, store repository.DinerStore, placesClient *places.Client, authService *auth.Service, googleAuth googleAuthenticator) *Handler {
	return &Handler{cfg: cfg, store: store, places: placesClient, authService: authService, googleAuth: googleAuth}
}

func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	visits, err := h.store.Visits(r.Context(), 6)
	if err != nil {
		h.error(w, "home visits", err)
		return
	}
	pickerTurn, err := h.store.PickerTurn(r.Context())
	if err != nil {
		h.error(w, "picker turn", err)
		return
	}
	h.render(w, "home", r, ui.PageData{Visits: visits, PickerTurn: pickerTurn})
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
	h.render(w, "restaurant", r, ui.PageData{
		Title:      restaurant.Name,
		Restaurant: restaurant,
		Visits:     visits,
		Notice:     googleRefreshNotice(r.URL.Query().Get("google_refresh")),
	})
}

func (h *Handler) EditRestaurantPage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid restaurant ID", http.StatusBadRequest)
		return
	}
	data, err := h.restaurantEditData(r, id)
	if err != nil {
		h.error(w, "restaurant edit", err)
		return
	}
	if data.Restaurant == nil {
		http.NotFound(w, r)
		return
	}
	h.render(w, "restaurant-edit", r, data)
}

func (h *Handler) Trophy(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.Stats(r.Context())
	if err != nil {
		h.error(w, "stats", err)
		return
	}
	pickerTurn, err := h.store.PickerTurn(r.Context())
	if err != nil {
		h.error(w, "picker turn", err)
		return
	}
	h.render(w, "trophy", r, ui.PageData{Title: "Trophy Case", Stats: stats, PickerTurn: pickerTurn})
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
	if err := input.Validate(); err != nil {
		h.renderLogError(w, r, err.Error())
		return
	}
	enrichedInput, err := placesync.EnrichVisitInput(r.Context(), h.places, input)
	if err != nil {
		slog.Warn("places details failed", "place_id", input.GooglePlaceID, "error", err)
	} else {
		input = enrichedInput
	}
	visitID, err := h.store.CreateVisit(r.Context(), input)
	if err != nil {
		h.renderLogError(w, r, err.Error())
		return
	}
	http.Redirect(w, r, "/dines#"+visitID.String(), http.StatusSeeOther)
}

func (h *Handler) EditVisitPage(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid visit ID", http.StatusBadRequest)
		return
	}
	data, err := h.visitEditData(r, id)
	if err != nil {
		h.error(w, "visit edit", err)
		return
	}
	if data.Visit == nil {
		http.NotFound(w, r)
		return
	}
	h.render(w, "visit-edit", r, data)
}

func (h *Handler) UpdateVisit(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid visit ID", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	input, err := h.visitInput(r)
	if err != nil {
		h.renderVisitEditError(w, r, id, err.Error())
		return
	}
	if err := h.store.UpdateVisit(r.Context(), id, input); err != nil {
		h.renderVisitEditError(w, r, id, err.Error())
		return
	}
	http.Redirect(w, r, "/dines#"+id.String(), http.StatusSeeOther)
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

func (h *Handler) DeleteRestaurant(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid restaurant ID", http.StatusBadRequest)
		return
	}
	deleted, err := h.store.DeleteRestaurantIfUnvisited(r.Context(), id)
	if err != nil {
		h.error(w, "delete restaurant", err)
		return
	}
	if !deleted {
		http.Error(w, "Restaurant has visits and cannot be removed from saved spots", http.StatusConflict)
		return
	}
	http.Redirect(w, r, searchRedirect(r.FormValue("q")), http.StatusSeeOther)
}

func (h *Handler) UpdateRestaurant(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid restaurant ID", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	input, err := h.restaurantInput(r)
	if err != nil {
		h.renderRestaurantEditError(w, r, id, err.Error())
		return
	}
	if err := h.store.UpdateRestaurant(r.Context(), id, input); err != nil {
		h.renderRestaurantEditError(w, r, id, err.Error())
		return
	}
	http.Redirect(w, r, "/restaurants/"+id.String(), http.StatusSeeOther)
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

func (h *Handler) RefreshRestaurantGoogle(w http.ResponseWriter, r *http.Request) {
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
	if h.places == nil || !h.places.Configured() {
		redirectRestaurantGoogleRefresh(w, r, id, "unconfigured")
		return
	}
	if restaurant.GooglePlaceID == nil || strings.TrimSpace(*restaurant.GooglePlaceID) == "" {
		redirectRestaurantGoogleRefresh(w, r, id, "missing-place-id")
		return
	}
	place, err := h.places.Details(r.Context(), *restaurant.GooglePlaceID)
	if err != nil {
		slog.Warn("places details refresh failed", "restaurant_id", id, "place_id", *restaurant.GooglePlaceID, "error", err)
		redirectRestaurantGoogleRefresh(w, r, id, "failed")
		return
	}
	if place == nil {
		redirectRestaurantGoogleRefresh(w, r, id, "failed")
		return
	}
	if err := h.store.UpdateRestaurantGoogleMetadata(r.Context(), id, placesync.MetadataFromPlace(*place)); err != nil {
		h.error(w, "refresh restaurant google metadata", err)
		return
	}
	redirectRestaurantGoogleRefresh(w, r, id, "updated")
}

func redirectRestaurantGoogleRefresh(w http.ResponseWriter, r *http.Request, id uuid.UUID, status string) {
	http.Redirect(w, r, "/restaurants/"+id.String()+"?google_refresh="+status, http.StatusSeeOther)
}

func googleRefreshNotice(status string) string {
	switch status {
	case "updated":
		return "Google info refreshed"
	case "failed":
		return "Google info refresh failed"
	case "missing-place-id":
		return "No Google Place ID saved for this restaurant"
	case "unconfigured":
		return "Google Places is not configured"
	default:
		return ""
	}
}

func searchRedirect(q string) string {
	if q == "" {
		return "/search"
	}
	return "/search?q=" + url.QueryEscape(q)
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
	var locationStatus string
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	near := strings.TrimSpace(r.URL.Query().Get("near"))
	lat, latErr := strconv.ParseFloat(r.URL.Query().Get("lat"), 64)
	lng, lngErr := strconv.ParseFloat(r.URL.Query().Get("lng"), 64)
	searched := (latErr == nil && lngErr == nil) || q != "" || near != ""
	if h.places.Configured() && latErr == nil && lngErr == nil {
		var foundPlaces []places.Place
		var err error
		if q == "" {
			foundPlaces, err = h.places.Nearby(r.Context(), lat, lng)
		} else {
			foundPlaces, err = h.places.TextSearchNear(r.Context(), q, lat, lng)
		}
		if err != nil {
			slog.Warn("nearby places failed", "error", err)
			locationStatus = "Nearby search failed. Try again, or search by city/address."
		} else {
			found = foundPlaces
		}
	} else if h.places.Configured() && (q != "" || near != "") {
		places, err := h.places.TextSearch(r.Context(), nearbyTextQuery(q, near))
		if err != nil {
			slog.Warn("nearby places search failed", "error", err)
			locationStatus = "Area search failed. Try again with a city, address, or neighborhood."
		} else {
			found = places
		}
	} else if searched && !h.places.Configured() {
		locationStatus = "Google Places is not configured for this environment, so nearby restaurant results cannot load."
	}
	if searched && h.places.Configured() && locationStatus == "" && len(found) == 0 {
		locationStatus = "No nearby restaurants found. Try a restaurant/cuisine search or a wider city/address search."
	}
	data := ui.PageData{
		Title:          "Nearby",
		Places:         found,
		Query:          q,
		LocationQuery:  near,
		LocationStatus: locationStatus,
	}
	if latErr == nil && lngErr == nil {
		data.HasLocation = true
		data.OriginLatitude = lat
		data.OriginLongitude = lng
	}
	h.render(w, "nearby", r, data)
}

func nearbyTextQuery(query, near string) string {
	switch {
	case query != "" && near != "":
		return query + " near " + near
	case near != "":
		return "restaurants near " + near
	default:
		return query
	}
}

func (h *Handler) LoginPage(w http.ResponseWriter, r *http.Request) {
	data := ui.PageData{Title: "Login", Query: r.URL.Query().Get("redirect")}
	if message := r.URL.Query().Get("message"); message != "" {
		data.Error = message
	}
	h.render(w, "login", r, data)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, googleLoginPath(r.FormValue("redirect")), http.StatusSeeOther)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(middleware.CookieName); err == nil && cookie.Value != "" && h.authService != nil {
		if err := h.authService.DeleteSession(r.Context(), cookie.Value); err != nil {
			slog.Error("delete session", "error", err)
		}
	}
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
	pickerTurn, err := h.store.PickerTurn(r.Context())
	if err != nil {
		return ui.PageData{}, err
	}
	return ui.PageData{
		Title:                   "Log a Dine",
		People:                  people,
		Tags:                    tags,
		Restaurants:             restaurants,
		PrefillName:             r.URL.Query().Get("restaurant_name"),
		PrefillAddress:          r.URL.Query().Get("address"),
		PrefillCity:             r.URL.Query().Get("city"),
		PrefillLatitude:         r.URL.Query().Get("latitude"),
		PrefillLongitude:        r.URL.Query().Get("longitude"),
		PrefillPhone:            r.URL.Query().Get("phone"),
		PrefillWebsite:          r.URL.Query().Get("website"),
		PrefillPlaceID:          r.URL.Query().Get("google_place_id"),
		PrefillGoogleRating:     r.URL.Query().Get("google_rating"),
		PrefillGooglePriceLevel: r.URL.Query().Get("google_price_level"),
		PrefillCategory:         r.URL.Query().Get("category"),
		PrefillPriceLevel:       prefillPrice,
		PrefillPickerID:         pickerTurn.NextPicker.ID.String(),
		PrefillRestaurantID:     r.URL.Query().Get("restaurant_id"),
	}, nil
}

func (h *Handler) visitInput(r *http.Request) (model.VisitInput, error) {
	visitedAt, err := apptime.ParseDatetimeLocal(r.FormValue("visited_at"))
	if err != nil {
		return model.VisitInput{}, err
	}
	pickerID, err := uuid.Parse(r.FormValue("picker_id"))
	if err != nil {
		return model.VisitInput{}, err
	}
	priceLevel, _ := strconv.Atoi(r.FormValue("price_level"))
	latitude, err := optionalFloat(r.FormValue("latitude"), "latitude")
	if err != nil {
		return model.VisitInput{}, err
	}
	longitude, err := optionalFloat(r.FormValue("longitude"), "longitude")
	if err != nil {
		return model.VisitInput{}, err
	}
	googleRating, err := optionalFloat(r.FormValue("google_rating"), "Google rating")
	if err != nil {
		return model.VisitInput{}, err
	}
	googlePriceLevel, err := optionalInt(r.FormValue("google_price_level"), "Google price level")
	if err != nil {
		return model.VisitInput{}, err
	}
	input := model.VisitInput{
		RestaurantName: r.FormValue("restaurant_name"),
		Address:        r.FormValue("address"),
		City:           r.FormValue("city"),
		GooglePlaceID:  r.FormValue("google_place_id"),
		GoogleMetadata: model.GoogleRestaurantMetadata{
			Latitude:         latitude,
			Longitude:        longitude,
			Phone:            r.FormValue("phone"),
			Website:          r.FormValue("website"),
			GoogleRating:     googleRating,
			GooglePriceLevel: googlePriceLevel,
		},
		Category:   r.FormValue("category"),
		IsChain:    r.FormValue("is_chain") == "true",
		VisitedAt:  visitedAt,
		PickerID:   pickerID,
		PriceLevel: priceLevel,
		Notes:      r.FormValue("notes"),
		NewTag:     r.FormValue("new_tag"),
		Ratings:    map[uuid.UUID]float64{},
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

func (h *Handler) restaurantInput(r *http.Request) (model.RestaurantInput, error) {
	googleRating, err := optionalFloat(r.FormValue("google_rating"), "Google rating")
	if err != nil {
		return model.RestaurantInput{}, err
	}
	googlePriceLevel, err := optionalInt(r.FormValue("google_price_level"), "Google price level")
	if err != nil {
		return model.RestaurantInput{}, err
	}
	input := model.RestaurantInput{
		Name:             r.FormValue("restaurant_name"),
		Address:          r.FormValue("address"),
		City:             r.FormValue("city"),
		Phone:            r.FormValue("phone"),
		Website:          r.FormValue("website"),
		GoogleRating:     googleRating,
		GooglePriceLevel: googlePriceLevel,
		Category:         r.FormValue("category"),
		IsChain:          r.FormValue("is_chain") == "true",
	}
	if err := input.Validate(); err != nil {
		return model.RestaurantInput{}, err
	}
	return input, nil
}

func (h *Handler) renderLogError(w http.ResponseWriter, r *http.Request, message string) {
	data, err := h.logData(r)
	if err != nil {
		h.error(w, "log error data", err)
		return
	}
	overlayLogPostForm(&data, r)
	data.Error = message
	h.render(w, "log", r, data)
}

func overlayLogPostForm(data *ui.PageData, r *http.Request) {
	data.PrefillRestaurantID = r.FormValue("restaurant_id")
	data.PrefillName = r.FormValue("restaurant_name")
	data.PrefillAddress = r.FormValue("address")
	data.PrefillCity = r.FormValue("city")
	data.PrefillLatitude = r.FormValue("latitude")
	data.PrefillLongitude = r.FormValue("longitude")
	data.PrefillPhone = r.FormValue("phone")
	data.PrefillWebsite = r.FormValue("website")
	data.PrefillPlaceID = r.FormValue("google_place_id")
	data.PrefillGoogleRating = r.FormValue("google_rating")
	data.PrefillGooglePriceLevel = r.FormValue("google_price_level")
	data.PrefillCategory = r.FormValue("category")
	data.NowLocal = r.FormValue("visited_at")
	data.PrefillPickerID = r.FormValue("picker_id")
	data.PrefillNotes = r.FormValue("notes")
	data.PrefillNewTag = r.FormValue("new_tag")
	data.PrefillIsChain = r.FormValue("is_chain") == "true"
	if priceLevel, err := strconv.Atoi(r.FormValue("price_level")); err == nil {
		data.PrefillPriceLevel = priceLevel
	}

	data.PrefillRatings = map[string]string{}
	for key, values := range r.PostForm {
		if len(values) == 0 || !strings.HasPrefix(key, "rating_") {
			continue
		}
		data.PrefillRatings[strings.TrimPrefix(key, "rating_")] = values[0]
	}

	data.PrefillTagIDs = map[string]bool{}
	for _, tagID := range r.PostForm["tag_id"] {
		data.PrefillTagIDs[tagID] = true
	}
}

func (h *Handler) visitEditData(r *http.Request, id uuid.UUID) (ui.PageData, error) {
	visit, err := h.store.Visit(r.Context(), id)
	if err != nil {
		return ui.PageData{}, err
	}
	if visit == nil {
		return ui.PageData{}, nil
	}
	people, err := h.store.People(r.Context())
	if err != nil {
		return ui.PageData{}, err
	}
	tags, err := h.store.Tags(r.Context())
	if err != nil {
		return ui.PageData{}, err
	}
	return ui.PageData{Title: "Edit Dine", Visit: visit, People: people, Tags: tags}, nil
}

func (h *Handler) restaurantEditData(r *http.Request, id uuid.UUID) (ui.PageData, error) {
	restaurant, err := h.store.Restaurant(r.Context(), id)
	if err != nil {
		return ui.PageData{}, err
	}
	if restaurant == nil {
		return ui.PageData{}, nil
	}
	return ui.PageData{Title: "Edit " + restaurant.Name, Restaurant: restaurant}, nil
}

func (h *Handler) renderVisitEditError(w http.ResponseWriter, r *http.Request, id uuid.UUID, message string) {
	data, err := h.visitEditData(r, id)
	if err != nil {
		h.error(w, "visit edit error data", err)
		return
	}
	if data.Visit == nil {
		http.NotFound(w, r)
		return
	}
	data.Error = message
	h.render(w, "visit-edit", r, data)
}

func (h *Handler) renderRestaurantEditError(w http.ResponseWriter, r *http.Request, id uuid.UUID, message string) {
	data, err := h.restaurantEditData(r, id)
	if err != nil {
		h.error(w, "restaurant edit error data", err)
		return
	}
	if data.Restaurant == nil {
		http.NotFound(w, r)
		return
	}
	data.Error = message
	h.render(w, "restaurant-edit", r, data)
}

func (h *Handler) render(w http.ResponseWriter, name string, r *http.Request, data ui.PageData) {
	data.Authenticated = middleware.IsAuthenticated(r, h.authService)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ui.Render(w, name, data); err != nil {
		slog.Error("render page", "page", name, "error", err)
	}
}

func optionalFloat(value, label string) (*float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil, fmt.Errorf("%s must be a number", label)
	}
	return &parsed, nil
}

func optionalInt(value, label string) (*int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil, fmt.Errorf("%s must be a number", label)
	}
	return &parsed, nil
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
