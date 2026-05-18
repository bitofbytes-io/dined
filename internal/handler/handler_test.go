package handler

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/google/uuid"
)

func TestNearbyTextQuery(t *testing.T) {
	tests := []struct {
		name  string
		query string
		near  string
		want  string
	}{
		{name: "restaurant near place", query: "tacos", near: "Brooklyn", want: "tacos near Brooklyn"},
		{name: "place only", near: "10001", want: "restaurants near 10001"},
		{name: "query only", query: "ramen", want: "ramen"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nearbyTextQuery(tt.query, tt.near); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGoogleRefreshNotice(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{status: "updated", want: "Google info refreshed"},
		{status: "failed", want: "Google info refresh failed"},
		{status: "missing-place-id", want: "No Google Place ID saved for this restaurant"},
		{status: "unconfigured", want: "Google Places is not configured"},
		{status: "ignored", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := googleRefreshNotice(tt.status); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestVisitInputParsesGoogleMetadata(t *testing.T) {
	personID := uuid.New()
	form := url.Values{
		"restaurant_name":             {"Google Grill"},
		"address":                     {"44 Search Street"},
		"city":                        {"Raleigh"},
		"latitude":                    {"35.779600"},
		"longitude":                   {"-78.638200"},
		"phone":                       {"919-555-0100"},
		"website":                     {"https://example.com"},
		"google_place_id":             {"google-grill-place"},
		"google_rating":               {"4.7"},
		"google_price_level":          {"3"},
		"category":                    {"American"},
		"visited_at":                  {apptime.FormatDatetimeLocal(time.Now())},
		"picker_id":                   {personID.String()},
		"price_level":                 {"2"},
		"rating_" + personID.String(): {"8"},
	}
	req := httptest.NewRequest("POST", "/visits", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatal(err)
	}

	input, err := (&Handler{}).visitInput(req)
	if err != nil {
		t.Fatal(err)
	}
	if input.GoogleMetadata.Latitude == nil || *input.GoogleMetadata.Latitude != 35.779600 || input.GoogleMetadata.Longitude == nil || *input.GoogleMetadata.Longitude != -78.638200 {
		t.Fatalf("coordinates = %#v/%#v", input.GoogleMetadata.Latitude, input.GoogleMetadata.Longitude)
	}
	if input.GoogleMetadata.Phone != "919-555-0100" || input.GoogleMetadata.Website != "https://example.com" {
		t.Fatalf("contact metadata = %q/%q", input.GoogleMetadata.Phone, input.GoogleMetadata.Website)
	}
	if input.GoogleMetadata.GoogleRating == nil || *input.GoogleMetadata.GoogleRating != 4.7 {
		t.Fatalf("google rating = %#v", input.GoogleMetadata.GoogleRating)
	}
	if input.GoogleMetadata.GooglePriceLevel == nil || *input.GoogleMetadata.GooglePriceLevel != 3 {
		t.Fatalf("google price level = %#v", input.GoogleMetadata.GooglePriceLevel)
	}
}

func TestRestaurantInputValidatesGoogleRating(t *testing.T) {
	form := url.Values{
		"restaurant_name": {"Hank's"},
		"google_rating":   {"6.1"},
	}
	req := httptest.NewRequest("POST", "/restaurants/id", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := req.ParseForm(); err != nil {
		t.Fatal(err)
	}

	_, err := (&Handler{}).restaurantInput(req)
	if err == nil {
		t.Fatal("expected validation error")
	}
}
