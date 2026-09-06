package handler

import (
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/placesync"
	"github.com/bitofbytes-io/dined/internal/ui"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

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
		Title:          restaurant.Name,
		Restaurant:     restaurant,
		Visits:         visits,
		ReadOnlyVisits: true,
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
	slog.Info("restaurant deleted", "restaurant_id", id)
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
	returnVisitID, err := h.restaurantReturnVisitID(r, id)
	if err != nil {
		h.error(w, "restaurant return visit", err)
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
	slog.Info("restaurant updated", "restaurant_id", id, "is_chain", input.IsChain)
	if returnVisitID != "" {
		http.Redirect(w, r, "/visits/"+returnVisitID+"/edit", http.StatusSeeOther)
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
	slog.Info("restaurant chain flag updated", "restaurant_id", id, "is_chain", isChain)
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
	returnVisitID, err := h.restaurantReturnVisitID(r, id)
	if err != nil {
		h.error(w, "restaurant google refresh return visit", err)
		return
	}
	if !h.placesConfigured() {
		redirectRestaurantGoogleRefresh(w, r, id, "unconfigured", returnVisitID)
		return
	}
	if restaurant.GooglePlaceID == nil || strings.TrimSpace(*restaurant.GooglePlaceID) == "" {
		redirectRestaurantGoogleRefresh(w, r, id, "missing-place-id", returnVisitID)
		return
	}
	place, err := h.places.Details(r.Context(), *restaurant.GooglePlaceID)
	if err != nil {
		slog.Warn("places details refresh failed", "restaurant_id", id, "place_id", *restaurant.GooglePlaceID, "error", err)
		redirectRestaurantGoogleRefresh(w, r, id, "failed", returnVisitID)
		return
	}
	if place == nil {
		redirectRestaurantGoogleRefresh(w, r, id, "failed", returnVisitID)
		return
	}
	if err := h.store.UpdateRestaurantGoogleMetadata(r.Context(), id, placesync.MetadataFromPlace(*place)); err != nil {
		h.error(w, "refresh restaurant google metadata", err)
		return
	}
	slog.Info("restaurant google metadata refreshed", "restaurant_id", id, "place_id", *restaurant.GooglePlaceID)
	redirectRestaurantGoogleRefresh(w, r, id, "updated", returnVisitID)
}

func redirectRestaurantGoogleRefresh(w http.ResponseWriter, r *http.Request, id uuid.UUID, status string, returnVisitID string) {
	query := url.Values{"google_refresh": {status}}
	if returnVisitID != "" {
		query.Set("return_visit_id", returnVisitID)
	}
	http.Redirect(w, r, "/restaurants/"+id.String()+"/edit?"+query.Encode(), http.StatusSeeOther)
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

func (h *Handler) restaurantEditData(r *http.Request, id uuid.UUID) (ui.PageData, error) {
	restaurant, err := h.store.Restaurant(r.Context(), id)
	if err != nil {
		return ui.PageData{}, err
	}
	if restaurant == nil {
		return ui.PageData{}, nil
	}
	returnVisitID, err := h.restaurantReturnVisitID(r, id)
	if err != nil {
		return ui.PageData{}, err
	}
	return ui.PageData{
		Title:         "Edit " + restaurant.Name,
		Restaurant:    restaurant,
		Notice:        googleRefreshNotice(r.URL.Query().Get("google_refresh")),
		ReturnVisitID: returnVisitID,
	}, nil
}

func (h *Handler) restaurantReturnVisitID(r *http.Request, restaurantID uuid.UUID) (string, error) {
	return h.validRestaurantReturnVisitID(r, restaurantID, r.FormValue("return_visit_id"))
}

func (h *Handler) validRestaurantReturnVisitID(r *http.Request, restaurantID uuid.UUID, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	visitID, err := uuid.Parse(value)
	if err != nil {
		return "", nil
	}
	visit, err := h.store.Visit(r.Context(), visitID)
	if err != nil {
		return "", err
	}
	if visit == nil || visit.Restaurant.ID != restaurantID {
		return "", nil
	}
	return visitID.String(), nil
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

func (h *Handler) searchResults(r *http.Request, restaurants []model.Restaurant) ([]ui.RestaurantResult, error) {
	results := make([]ui.RestaurantResult, 0, len(restaurants))
	for _, restaurant := range restaurants {
		visits, err := h.store.RestaurantVisitSummaries(r.Context(), restaurant.ID)
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
