package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/placesync"
	"github.com/bitofbytes-io/dined/internal/ui"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

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
	slog.Info("visit created", "visit_id", visitID, "picker_id", input.PickerID, "rating_count", len(input.Ratings), "tag_count", len(input.TagIDs), "photo_count", len(input.Photos))
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
	slog.Info("visit updated", "visit_id", id, "picker_id", input.PickerID, "rating_count", len(input.Ratings), "tag_count", len(input.TagIDs), "photo_count", len(input.Photos))
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
	slog.Info("visit deleted", "visit_id", id)
	http.Redirect(w, r, "/dines", http.StatusSeeOther)
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
	latitude, err := optionalCoordinate(r.FormValue("latitude"), "latitude", -90, 90)
	if err != nil {
		return model.VisitInput{}, err
	}
	longitude, err := optionalCoordinate(r.FormValue("longitude"), "longitude", -180, 180)
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
	for _, value := range r.PostForm["keep_photo_id"] {
		photoID, err := uuid.Parse(value)
		if err != nil {
			return model.VisitInput{}, err
		}
		input.KeepPhotoIDs = append(input.KeepPhotoIDs, photoID)
	}
	for _, value := range r.PostForm["photo_data_uri"] {
		if strings.TrimSpace(value) == "" {
			continue
		}
		input.Photos = append(input.Photos, model.VisitPhotoInput{DataURI: value})
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
	data.PrefillPhotoDataURIs = append([]string(nil), r.PostForm["photo_data_uri"]...)
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
