package handler

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/ui"
)

const maxPlacesQueryLength = 160

func searchRedirect(q string) string {
	if q == "" {
		return "/search"
	}
	return "/search?q=" + url.QueryEscape(q)
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	q, ok := boundedSearchParam(w, r.URL.Query().Get("q"), "query")
	if !ok {
		return
	}
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
	if q != "" && h.placesConfigured() {
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
	q, ok := boundedSearchParam(w, r.URL.Query().Get("q"), "query")
	if !ok {
		return
	}
	near, ok := boundedSearchParam(w, r.URL.Query().Get("near"), "near")
	if !ok {
		return
	}
	lat, lng, hasCoordinates, ok := coordinatesFromQuery(w, r)
	if !ok {
		return
	}
	searched := hasCoordinates || q != "" || near != ""
	if h.placesConfigured() && hasCoordinates {
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
	} else if h.placesConfigured() && (q != "" || near != "") {
		places, err := h.places.TextSearch(r.Context(), nearbyTextQuery(q, near))
		if err != nil {
			slog.Warn("nearby places search failed", "error", err)
			locationStatus = "Area search failed. Try again with a city, address, or neighborhood."
		} else {
			found = places
		}
	} else if searched && !h.placesConfigured() {
		locationStatus = "Google Places is not configured for this environment, so nearby restaurant results cannot load."
	}
	if searched && h.placesConfigured() && locationStatus == "" && len(found) == 0 {
		locationStatus = "No nearby restaurants found. Try a restaurant/cuisine search or a wider city/address search."
	}
	data := ui.PageData{
		Title:          "Nearby",
		Places:         found,
		Query:          q,
		LocationQuery:  near,
		LocationStatus: locationStatus,
	}
	if hasCoordinates {
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

func boundedSearchParam(w http.ResponseWriter, value string, label string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) > maxPlacesQueryLength {
		http.Error(w, label+" is too long", http.StatusBadRequest)
		return "", false
	}
	return value, true
}

func coordinatesFromQuery(w http.ResponseWriter, r *http.Request) (float64, float64, bool, bool) {
	latValue := strings.TrimSpace(r.URL.Query().Get("lat"))
	lngValue := strings.TrimSpace(r.URL.Query().Get("lng"))
	if latValue == "" && lngValue == "" {
		return 0, 0, false, true
	}
	if latValue == "" || lngValue == "" {
		http.Error(w, "latitude and longitude are required together", http.StatusBadRequest)
		return 0, 0, false, false
	}

	latitude, err := parseCoordinate(latValue, "latitude", -90, 90)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return 0, 0, false, false
	}
	longitude, err := parseCoordinate(lngValue, "longitude", -180, 180)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return 0, 0, false, false
	}

	return latitude, longitude, true, true
}
