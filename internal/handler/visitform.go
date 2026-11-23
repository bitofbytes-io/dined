package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/ui"
	"github.com/google/uuid"
)

const ratingFieldPrefix = "rating_"

// errInvalidPicker means the picker form value is neither Everybody nor a UUID.
var errInvalidPicker = errors.New("invalid picker")

// visitInput parses the dine form of a request whose form has been parsed.
func (h *Handler) visitInput(r *http.Request) (model.VisitInput, error) {
	return parseVisitForm(r.Form, r.PostForm)
}

// parseVisitForm turns a submitted dine form into a VisitInput. Single-value
// fields come from form (body values win over the query string, as with
// http.Request.FormValue); ratings, tags and photos are repeated or dynamic
// fields and are read from postForm only.
func parseVisitForm(form, postForm url.Values) (model.VisitInput, error) {
	visitedAt, err := apptime.ParseDatetimeLocal(form.Get("visited_at"))
	if err != nil {
		return model.VisitInput{}, err
	}
	pickerID, err := parsePickerID(form.Get("picker_id"))
	if err != nil {
		return model.VisitInput{}, err
	}
	priceLevel, _ := strconv.Atoi(form.Get("price_level"))
	latitude, err := optionalCoordinate(form.Get("latitude"), "latitude", -90, 90)
	if err != nil {
		return model.VisitInput{}, err
	}
	longitude, err := optionalCoordinate(form.Get("longitude"), "longitude", -180, 180)
	if err != nil {
		return model.VisitInput{}, err
	}
	googleRating, err := optionalFloat(form.Get("google_rating"), "Google rating")
	if err != nil {
		return model.VisitInput{}, err
	}
	googlePriceLevel, err := optionalInt(form.Get("google_price_level"), "Google price level")
	if err != nil {
		return model.VisitInput{}, err
	}
	input := model.VisitInput{
		RestaurantName: form.Get("restaurant_name"),
		Address:        form.Get("address"),
		City:           form.Get("city"),
		GooglePlaceID:  form.Get("google_place_id"),
		GoogleMetadata: model.GoogleRestaurantMetadata{
			Latitude:         latitude,
			Longitude:        longitude,
			Phone:            form.Get("phone"),
			Website:          form.Get("website"),
			GoogleRating:     googleRating,
			GooglePriceLevel: googlePriceLevel,
		},
		Category:   form.Get("category"),
		IsChain:    form.Get("is_chain") == "true",
		VisitedAt:  visitedAt,
		PickerID:   pickerID,
		PriceLevel: priceLevel,
		Notes:      form.Get("notes"),
		NewTag:     form.Get("new_tag"),
		Ratings:    map[uuid.UUID]float64{},
	}
	if restaurantID := form.Get("restaurant_id"); restaurantID != "" {
		id, err := uuid.Parse(restaurantID)
		if err != nil {
			return model.VisitInput{}, err
		}
		input.RestaurantID = &id
	}
	for key, values := range postForm {
		if len(values) == 0 || !strings.HasPrefix(key, ratingFieldPrefix) || values[0] == "" {
			continue
		}
		personID, err := uuid.Parse(strings.TrimPrefix(key, ratingFieldPrefix))
		if err != nil {
			return model.VisitInput{}, err
		}
		score, err := strconv.ParseFloat(values[0], 64)
		if err != nil {
			return model.VisitInput{}, err
		}
		input.Ratings[personID] = score
	}
	for _, value := range postForm["tag_id"] {
		tagID, err := uuid.Parse(value)
		if err != nil {
			return model.VisitInput{}, err
		}
		input.TagIDs = append(input.TagIDs, tagID)
	}
	for _, value := range postForm["keep_photo_id"] {
		photoID, err := uuid.Parse(value)
		if err != nil {
			return model.VisitInput{}, err
		}
		input.KeepPhotoIDs = append(input.KeepPhotoIDs, photoID)
	}
	for _, value := range postForm["photo_data_uri"] {
		if strings.TrimSpace(value) == "" {
			continue
		}
		input.Photos = append(input.Photos, model.VisitPhotoInput{DataURI: value})
	}
	return input, nil
}

// parsePickerID reads the picker form value: Everybody (or no value) is nil,
// and anything else must be a person's UUID.
func parsePickerID(value string) (*uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == ui.EverybodyPickerValue {
		return nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return nil, errInvalidPicker
	}
	return &id, nil
}

// pickerLogValue names a picker in logs without dereferencing a nil (Everybody) picker.
func pickerLogValue(id *uuid.UUID) string {
	if id == nil {
		return ui.EverybodyPickerValue
	}
	return id.String()
}
