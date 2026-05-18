package places

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestFieldMasksAreScopedToEndpointShape(t *testing.T) {
	if strings.Contains(searchFieldMask, ",id") || strings.Contains(searchFieldMask, ",displayName") {
		t.Fatalf("search field mask must only use places.* paths: %s", searchFieldMask)
	}
	if strings.Contains(detailsFieldMask, "places.") {
		t.Fatalf("details field mask must use unprefixed Place fields: %s", detailsFieldMask)
	}
}

func TestTextSearchNearAddsLocationBias(t *testing.T) {
	body := textSearchNearBody("tacos", 40.7128, -74.0060)

	bias := body["locationBias"].(map[string]any)
	circle := bias["circle"].(map[string]any)
	center := circle["center"].(map[string]float64)
	if center["latitude"] != 40.7128 || center["longitude"] != -74.0060 {
		t.Fatalf("unexpected center: %#v", center)
	}
	if circle["radius"] != 8047.0 {
		t.Fatalf("unexpected radius: %#v", circle["radius"])
	}
}

func TestPriceLevelNumber(t *testing.T) {
	if got := PriceLevelNumber("PRICE_LEVEL_MODERATE"); got != 2 {
		t.Fatalf("got %d", got)
	}
	if got := PriceLevelNumber("PRICE_LEVEL_UNSPECIFIED"); got != 0 {
		t.Fatalf("got %d", got)
	}
}

func TestPlaceDecodesGoogleMetadataFields(t *testing.T) {
	var place Place
	data := []byte(`{
		"id": "ChIJt4Y3X4v1rIkRikvaj7MLK-M",
		"displayName": {"text": "Tupelo Honey Southern Kitchen & Bar"},
		"formattedAddress": "425 Oberlin Rd, Raleigh, NC 27605, USA",
		"location": {"latitude": 35.7903375, "longitude": -78.6631725},
		"nationalPhoneNumber": "(919) 723-9353",
		"websiteUri": "https://tupelohoneycafe.com/restaurant/raleigh/",
		"rating": 4.3,
		"priceLevel": "PRICE_LEVEL_MODERATE"
	}`)

	if err := json.Unmarshal(data, &place); err != nil {
		t.Fatal(err)
	}
	if place.Phone != "(919) 723-9353" {
		t.Fatalf("Phone = %q", place.Phone)
	}
	if place.Website != "https://tupelohoneycafe.com/restaurant/raleigh/" {
		t.Fatalf("Website = %q", place.Website)
	}
	if place.Rating != 4.3 {
		t.Fatalf("Rating = %f", place.Rating)
	}
	if got := PriceLevelNumber(place.PriceLevel); got != 2 {
		t.Fatalf("PriceLevel = %d", got)
	}
	if place.Location.Latitude != 35.7903375 || place.Location.Longitude != -78.6631725 {
		t.Fatalf("Location = %#v", place.Location)
	}
}

func TestCategoryMapsRestaurantTypes(t *testing.T) {
	place := Place{Types: []string{"brunch_restaurant", "american_restaurant", "restaurant"}}

	if got := Category(place); got != "American" {
		t.Fatalf("Category = %q, want American", got)
	}
}

func TestCityUsesLocalityAddressComponent(t *testing.T) {
	place := Place{AddressComponents: []AddressComponent{
		{LongText: "Wake County", ShortText: "Wake", Types: []string{"administrative_area_level_2"}},
		{LongText: "Apex", ShortText: "Apex", Types: []string{"locality", "political"}},
	}}

	if got := City(place); got != "Apex" {
		t.Fatalf("got %q, want Apex", got)
	}
}

func TestCityFallsBackToLocalityShortText(t *testing.T) {
	place := Place{AddressComponents: []AddressComponent{
		{ShortText: "Cary", Types: []string{"locality", "political"}},
	}}

	if got := City(place); got != "Cary" {
		t.Fatalf("got %q, want Cary", got)
	}
}

func TestCityRequiresLocality(t *testing.T) {
	place := Place{AddressComponents: []AddressComponent{
		{LongText: "North Carolina", ShortText: "NC", Types: []string{"administrative_area_level_1", "political"}},
	}}

	if got := City(place); got != "" {
		t.Fatalf("got %q, want empty city", got)
	}
}

func TestDistanceMiles(t *testing.T) {
	place := Place{Location: Location{Latitude: 35.7327, Longitude: -78.8503}}
	if got := DistanceMiles(35.7327, -78.8503, place); got != 0 {
		t.Fatalf("same coordinates got %f", got)
	}

	place.Location = Location{Latitude: 35.7427, Longitude: -78.8503}
	if got := DistanceMiles(35.7327, -78.8503, place); math.Abs(got-0.691) > 0.01 {
		t.Fatalf("got %f miles", got)
	}
}
