package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/ui"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

const (
	// wantToTrySheetHeader marks a save sent by the save sheet's script, which
	// wants a status rather than a redirect so the page it is on stays put.
	wantToTrySheetHeader = "X-Dined-Want-To-Try-Sheet"
	// invalidSuggesterMessage answers a suggester that is neither Everybody nor one of the people.
	invalidSuggesterMessage    = "Whose idea? must be Everybody or one of the family."
	saveWantToTryFailedMessage = "Could not save this place. Please try again."
)

func (h *Handler) WantToTry(w http.ResponseWriter, r *http.Request) {
	items, err := h.store.WantToTry(r.Context())
	if err != nil {
		h.error(w, "want to try", err)
		return
	}
	notice, err := h.wantToTryNotice(r)
	if err != nil {
		h.error(w, "want to try notice", err)
		return
	}
	h.render(w, "want-to-try", r, ui.PageData{Title: "Want to Try", WantToTryItems: items, Notice: notice})
}

// wantToTryNotice confirms the save or dine that led back to the list.
func (h *Handler) wantToTryNotice(r *http.Request) (string, error) {
	query := r.URL.Query()
	for _, key := range []string{"logged", "saved"} {
		id, err := uuid.Parse(query.Get(key))
		if err != nil {
			continue
		}
		item, err := h.store.WantToTryItem(r.Context(), id)
		if err != nil || item == nil {
			return "", err
		}
		if key == "logged" && item.DoneAt != nil {
			return "Logged " + item.Restaurant.Name + ". It's off the Want to Try list.", nil
		}
		if key == "saved" && item.DoneAt == nil {
			return "Saved " + item.Restaurant.Name + " to Want to Try.", nil
		}
	}
	return "", nil
}

// NewWantToTryPage is the save form for browsers without the save sheet.
func (h *Handler) NewWantToTryPage(w http.ResponseWriter, r *http.Request) {
	data, ok := h.wantToTryFormData(w, r, r.URL.Query())
	if !ok {
		return
	}
	data.PrefillPickerID = ui.EverybodyPickerValue
	h.render(w, "want-to-try-new", r, data)
}

func (h *Handler) SaveWantToTry(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form", http.StatusBadRequest)
		return
	}
	input, err := wantToTryInput(r)
	if errors.Is(err, errInvalidPicker) {
		http.Error(w, invalidSuggesterMessage, http.StatusBadRequest)
		return
	}
	if err == nil {
		err = input.Validate()
	}
	if err != nil {
		h.wantToTrySaveError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	id, err := h.store.SaveWantToTry(r.Context(), input)
	if errors.Is(err, model.ErrUnknownPicker) {
		http.Error(w, invalidSuggesterMessage, http.StatusBadRequest)
		return
	}
	if errors.Is(err, model.ErrRestaurantNotFound) {
		http.Error(w, "Restaurant not found", http.StatusNotFound)
		return
	}
	var placeConflict *model.PlaceConflictError
	if errors.As(err, &placeConflict) {
		slog.Warn("save want to try place conflict", "place_id", input.GooglePlaceID, "error", err)
		h.wantToTrySaveError(w, r, http.StatusConflict, placeConflict.Restaurant+" is linked to a different Google place.")
		return
	}
	if err != nil {
		slog.Error("save want to try", "error", err)
		h.wantToTrySaveError(w, r, http.StatusInternalServerError, saveWantToTryFailedMessage)
		return
	}
	slog.Info("want to try saved", "want_to_try_id", id, "suggested_by", pickerLogValue(input.SuggestedByID))
	if r.Header.Get(wantToTrySheetHeader) != "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, "/want-to-try?saved="+id.String(), http.StatusSeeOther)
}

// wantToTrySaveError answers the save sheet with the message alone, and
// shows the save form page again with the message otherwise.
func (h *Handler) wantToTrySaveError(w http.ResponseWriter, r *http.Request, status int, message string) {
	if r.Header.Get(wantToTrySheetHeader) != "" {
		http.Error(w, message, status)
		return
	}
	data, ok := h.wantToTryFormData(w, r, r.PostForm)
	if !ok {
		return
	}
	data.Error = message
	data.PrefillPickerID = r.PostFormValue("suggested_by")
	data.PrefillNotes = r.PostFormValue("note")
	w.WriteHeader(status)
	h.render(w, "want-to-try-new", r, data)
}

func (h *Handler) DeleteWantToTry(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid Want to Try ID", http.StatusBadRequest)
		return
	}
	if err := h.store.DeleteWantToTry(r.Context(), id); err != nil {
		h.error(w, "delete want to try", err)
		return
	}
	slog.Info("want to try removed", "want_to_try_id", id)
	http.Redirect(w, r, "/want-to-try", http.StatusSeeOther)
}

// wantToTryFormData describes the restaurant a save form page names: a saved
// restaurant by ID, or a place by name. It answers the request itself and
// returns false when there is no such restaurant.
func (h *Handler) wantToTryFormData(w http.ResponseWriter, r *http.Request, values url.Values) (ui.PageData, bool) {
	form := url.Values{}
	for _, name := range []string{"restaurant_id", "restaurant_name", "address", "city", "google_place_id", "latitude", "longitude", "phone", "website", "google_rating", "google_price_level", "category"} {
		if value := strings.TrimSpace(values.Get(name)); value != "" {
			form.Set(name, value)
		}
	}
	people, err := h.store.People(r.Context())
	if err != nil {
		h.error(w, "want to try people", err)
		return ui.PageData{}, false
	}
	data := ui.PageData{
		PrefillName:        form.Get("restaurant_name"),
		People:             people,
		PrefillAddress:     form.Get("address"),
		WantToTryForm:      form,
		WantToTryCancelURL: localPath(values.Get("return_to"), "/want-to-try"),
	}
	if value := form.Get("restaurant_id"); value != "" {
		id, err := uuid.Parse(value)
		if err != nil {
			http.Error(w, "Invalid restaurant ID", http.StatusBadRequest)
			return ui.PageData{}, false
		}
		restaurant, err := h.store.Restaurant(r.Context(), id)
		if err != nil {
			h.error(w, "want to try restaurant", err)
			return ui.PageData{}, false
		}
		if restaurant == nil {
			http.NotFound(w, r)
			return ui.PageData{}, false
		}
		data.PrefillName = restaurant.Name
		data.PrefillAddress = valueOrEmpty(restaurant.Address)
		data.WantToTryForm = url.Values{"restaurant_id": {id.String()}}
	}
	if data.PrefillName == "" {
		http.Error(w, "restaurant is required", http.StatusBadRequest)
		return ui.PageData{}, false
	}
	data.Title = "Add " + data.PrefillName + " to Want to Try"
	return data, true
}

func wantToTryInput(r *http.Request) (model.WantToTryInput, error) {
	suggestedByID, err := parsePickerID(r.PostFormValue("suggested_by"))
	if err != nil {
		return model.WantToTryInput{}, err
	}
	latitude, err := optionalCoordinate(r.PostFormValue("latitude"), "latitude", -90, 90)
	if err != nil {
		return model.WantToTryInput{}, err
	}
	longitude, err := optionalCoordinate(r.PostFormValue("longitude"), "longitude", -180, 180)
	if err != nil {
		return model.WantToTryInput{}, err
	}
	googleRating, err := optionalFloat(r.PostFormValue("google_rating"), "Google rating")
	if err != nil {
		return model.WantToTryInput{}, err
	}
	googlePriceLevel, err := optionalInt(r.PostFormValue("google_price_level"), "Google price level")
	if err != nil {
		return model.WantToTryInput{}, err
	}
	input := model.WantToTryInput{
		RestaurantName: r.PostFormValue("restaurant_name"),
		Address:        r.PostFormValue("address"),
		City:           r.PostFormValue("city"),
		GooglePlaceID:  r.PostFormValue("google_place_id"),
		GoogleMetadata: model.GoogleRestaurantMetadata{
			Latitude:         latitude,
			Longitude:        longitude,
			Phone:            r.PostFormValue("phone"),
			Website:          r.PostFormValue("website"),
			GoogleRating:     googleRating,
			GooglePriceLevel: googlePriceLevel,
		},
		Category:      r.PostFormValue("category"),
		SuggestedByID: suggestedByID,
		Note:          r.PostFormValue("note"),
	}
	if value := strings.TrimSpace(r.PostFormValue("restaurant_id")); value != "" {
		id, err := uuid.Parse(value)
		if err != nil {
			return model.WantToTryInput{}, errors.New("invalid restaurant")
		}
		input.RestaurantID = &id
	}
	return input, nil
}

// addWantToTrySheet marks which places on the page are already on the Want
// to Try list and adds the save sheet. A failed lookup only leaves every
// place offering "+ Want to Try", which saves without harm.
func (h *Handler) addWantToTrySheet(r *http.Request, data *ui.PageData) {
	data.WantToTrySheet = true
	if data.People == nil {
		people, err := h.store.People(r.Context())
		if err != nil {
			slog.Warn("want to try people", "error", err)
		}
		data.People = people
	}
	items, err := h.store.WantToTry(r.Context())
	if err != nil {
		slog.Warn("want to try keys", "error", err)
		return
	}
	data.WantToTryKeys = make(map[string]bool, 2*len(items))
	for _, item := range items {
		data.WantToTryKeys[item.Restaurant.ID.String()] = true
		if placeID := strings.TrimSpace(valueOrEmpty(item.Restaurant.GooglePlaceID)); placeID != "" {
			data.WantToTryKeys[placeID] = true
		}
	}
}

// wantToTryCount counts the open Want to Try items for the home teaser,
// which a failed count only hides.
func (h *Handler) wantToTryCount(r *http.Request) int {
	items, err := h.store.WantToTry(r.Context())
	if err != nil {
		slog.Warn("want to try count", "error", err)
		return 0
	}
	return len(items)
}

// addWantToTryLogPrefill fills a log form from the open Want to Try item it
// was opened for: the restaurant, and Picked by set to whoever suggested it.
func (h *Handler) addWantToTryLogPrefill(r *http.Request, data *ui.PageData) error {
	id, err := uuid.Parse(strings.TrimSpace(r.FormValue("want_to_try")))
	if err != nil {
		return nil
	}
	item, err := h.store.WantToTryItem(r.Context(), id)
	if err != nil || item == nil || item.DoneAt != nil {
		return err
	}
	data.WantToTryItem = item
	data.PrefillRestaurantID = item.Restaurant.ID.String()
	data.PrefillName = item.Restaurant.Name
	data.PrefillAddress = valueOrEmpty(item.Restaurant.Address)
	data.PrefillCity = valueOrEmpty(item.Restaurant.City)
	data.PrefillCategory = valueOrEmpty(item.Restaurant.Category)
	data.PrefillPickerID = ui.EverybodyPickerValue
	if item.SuggestedBy != nil {
		data.PrefillPickerID = item.SuggestedBy.ID.String()
	}
	if item.Restaurant.GooglePriceLevel != nil {
		data.PrefillPriceLevel = *item.Restaurant.GooglePriceLevel
	}
	return nil
}

// createdVisitURL is where saving a new dine lands: back on the Want to Try
// list when the dine was logged at the item it was opened from, which is now
// off the list, and on the dine in /dines otherwise (say, when the form was
// switched to another restaurant).
func (h *Handler) createdVisitURL(r *http.Request, visitID uuid.UUID) string {
	if id, err := uuid.Parse(strings.TrimSpace(r.PostFormValue("want_to_try"))); err == nil && h.visitClearedWantToTry(r, visitID, id) {
		return "/want-to-try?logged=" + id.String()
	}
	return h.dinesVisitURL(r, visitID)
}

// visitClearedWantToTry reports whether the visit is at the item's restaurant
// and the item is off the list.
func (h *Handler) visitClearedWantToTry(r *http.Request, visitID, itemID uuid.UUID) bool {
	item, err := h.store.WantToTryItem(r.Context(), itemID)
	if err != nil || item == nil || item.DoneAt == nil {
		if err != nil {
			slog.Warn("want to try after visit", "want_to_try_id", itemID, "error", err)
		}
		return false
	}
	visit, err := h.store.Visit(r.Context(), visitID)
	if err != nil {
		slog.Warn("visit after want to try", "visit_id", visitID, "error", err)
		return false
	}
	return visit != nil && visit.Restaurant.ID == item.Restaurant.ID
}

// localPath returns value when it is a path on this site, else fallback.
// Browsers drop tabs and newlines from URLs and read backslashes as slashes,
// so values with those could still reach another site.
func localPath(value, fallback string) string {
	value = strings.TrimSpace(value)
	unsafe := strings.ContainsFunc(value, func(c rune) bool { return c < 0x20 || c == 0x7f || c == '\\' })
	if unsafe || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return fallback
	}
	return value
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
