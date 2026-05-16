package places

import (
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
