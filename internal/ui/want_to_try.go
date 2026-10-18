package ui

import (
	"fmt"
	"html/template"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
)

// WantToTryLink is the data for the "want-to-try-link" template: the
// "+ Want to Try" link for one place, or a note that it is already listed.
type WantToTryLink struct {
	On      bool
	Name    string
	Address string
	// URL opens the save form page; app.js reads its query to fill the save sheet.
	URL string
}

// OnWantToTry reports whether the place or restaurant with this key (a Google
// place ID or a restaurant ID) is on the Want to Try list.
func (d PageData) OnWantToTry(key string) bool {
	return d.WantToTryKeys[key]
}

func wantToTryFuncs() template.FuncMap {
	return template.FuncMap{
		"wantToTryPlaceLink":      wantToTryPlaceLink,
		"wantToTryRestaurantLink": wantToTryRestaurantLink,
		"wantToTryIdea":           wantToTryIdea,
		"wantToTryPlaceLine":      wantToTryPlaceLine,
		"wantToTryNoteMax": func() int {
			return model.MaxWantToTryNoteLength
		},
		"wantToTryAge": func(t time.Time) string {
			return wantToTryAge(t, time.Now())
		},
	}
}

// wantToTryPlaceLink links a Google place to the Want to Try save form with
// the same details a "Log this dine" link carries.
func wantToTryPlaceLink(data PageData, place places.Place) WantToTryLink {
	values := url.Values{}
	values.Set("restaurant_name", place.DisplayName.Text)
	values.Set("address", place.Address)
	values.Set("city", places.City(place))
	values.Set("google_place_id", place.ID)
	if place.Phone != "" {
		values.Set("phone", place.Phone)
	}
	if place.Website != "" {
		values.Set("website", place.Website)
	}
	if place.Rating > 0 {
		values.Set("google_rating", fmt.Sprintf("%.1f", place.Rating))
	}
	if priceLevel := places.PriceLevelNumber(place.PriceLevel); priceLevel > 0 {
		values.Set("google_price_level", strconv.Itoa(priceLevel))
	}
	if place.Location.Latitude != 0 || place.Location.Longitude != 0 {
		values.Set("latitude", fmt.Sprintf("%.6f", place.Location.Latitude))
		values.Set("longitude", fmt.Sprintf("%.6f", place.Location.Longitude))
	}
	values.Set("category", places.Category(place))
	return wantToTryLink(data, place.ID, place.DisplayName.Text, place.Address, values)
}

// wantToTryRestaurantLink links a saved restaurant to the Want to Try save form.
func wantToTryRestaurantLink(data PageData, restaurant model.Restaurant) WantToTryLink {
	values := url.Values{}
	values.Set("restaurant_id", restaurant.ID.String())
	return wantToTryLink(data, restaurant.ID.String(), restaurant.Name, valueOrEmpty(restaurant.Address), values)
}

func wantToTryLink(data PageData, key, name, address string, values url.Values) WantToTryLink {
	if data.RequestURI != "" {
		values.Set("return_to", data.RequestURI)
	}
	return WantToTryLink{
		On:      data.OnWantToTry(key),
		Name:    name,
		Address: address,
		URL:     "/want-to-try/new?" + values.Encode(),
	}
}

// wantToTryIdea says whose idea a Want to Try item was; nil means Everybody's.
func wantToTryIdea(person *model.Person) string {
	return pickerName(person) + "'s idea"
}

// wantToTryPlaceLine describes where a listed restaurant is: its category and
// city, or its address when it has neither.
func wantToTryPlaceLine(restaurant model.Restaurant) string {
	var parts []string
	for _, value := range []*string{restaurant.Category, restaurant.City} {
		if text := strings.TrimSpace(valueOrEmpty(value)); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return strings.TrimSpace(valueOrEmpty(restaurant.Address))
	}
	return strings.Join(parts, " · ")
}

// wantToTryAge says roughly how long ago a place went on the list.
func wantToTryAge(added, now time.Time) string {
	days := int(now.Sub(added).Hours() / 24)
	switch {
	case days < 1:
		return "today"
	case days == 1:
		return "yesterday"
	case days < 7:
		return strconv.Itoa(days) + " days ago"
	case days < 30:
		return plural(days/7, "week") + " ago"
	case days < 365:
		return plural(days/30, "month") + " ago"
	default:
		return plural(days/365, "year") + " ago"
	}
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
