package places

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/url"
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
	tests := []struct {
		name  string
		types []string
		want  string
	}{
		{
			name:  "american",
			types: []string{"brunch_restaurant", "american_restaurant", "restaurant"},
			want:  "American",
		},
		{
			name:  "korean",
			types: []string{"korean_restaurant", "restaurant"},
			want:  "Korean",
		},
		{
			name:  "cuban",
			types: []string{"cuban_restaurant", "restaurant"},
			want:  "Cuban",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			place := Place{Types: test.types}
			if got := Category(place); got != test.want {
				t.Fatalf("Category = %q, want %s", got, test.want)
			}
		})
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

func TestStaticMapURLRequiresMarkers(t *testing.T) {
	if _, err := staticMapURL("api-key", StaticMapRequest{}); err == nil {
		t.Fatal("expected marker validation error")
	}
}

func TestStaticMapURLCentersSingleMarker(t *testing.T) {
	mapURL, err := staticMapURL("api-key", StaticMapRequest{Markers: []StaticMapMarker{{Latitude: 35.7796, Longitude: -78.6382}}})
	if err != nil {
		t.Fatal(err)
	}
	values := staticMapQuery(t, mapURL)

	if values.Get("size") != "640x360" || values.Get("scale") != "2" || values.Get("maptype") != "roadmap" {
		t.Fatalf("unexpected static map basics: %s", values.Encode())
	}
	if values.Get("key") != "api-key" {
		t.Fatalf("key = %q", values.Get("key"))
	}
	if values.Get("center") != "35.779600,-78.638200" || values.Get("zoom") != "13" {
		t.Fatalf("single marker center/zoom = %q/%q", values.Get("center"), values.Get("zoom"))
	}
	if got := values.Get("markers"); got != "color:red|35.779600,-78.638200" {
		t.Fatalf("markers = %q", got)
	}
}

func TestStaticMapURLOmitsCenterAndZoomForMultipleMarkers(t *testing.T) {
	mapURL, err := staticMapURL("api-key", StaticMapRequest{Markers: []StaticMapMarker{
		{Latitude: 35.7796, Longitude: -78.6382},
		{Latitude: 35.7327, Longitude: -78.8503},
	}})
	if err != nil {
		t.Fatal(err)
	}
	values := staticMapQuery(t, mapURL)

	if values.Has("center") || values.Has("zoom") {
		t.Fatalf("multiple markers should rely on implicit positioning, got %s", values.Encode())
	}
	if got := values.Get("markers"); got != "color:red|35.779600,-78.638200|35.732700,-78.850300" {
		t.Fatalf("markers = %q", got)
	}
}

func TestStaticMapURLUsesDeterministicViewport(t *testing.T) {
	mapURL, err := staticMapURL("api-key", StaticMapRequest{
		Markers: []StaticMapMarker{
			{Latitude: 35.7796, Longitude: -78.6382},
			{Latitude: 35.7327, Longitude: -78.8503},
		},
		Viewport: &StaticMapViewport{Latitude: 35.756172, Longitude: -78.744450, Zoom: 11},
	})
	if err != nil {
		t.Fatal(err)
	}
	values := staticMapQuery(t, mapURL)

	if values.Get("center") != "35.756172,-78.744450" || values.Get("zoom") != "11" {
		t.Fatalf("viewport center/zoom = %q/%q", values.Get("center"), values.Get("zoom"))
	}
}

func TestStaticMapURLCapsMarkers(t *testing.T) {
	markers := make([]StaticMapMarker, staticMapMaxMarkers+25)
	for i := range markers {
		markers[i] = StaticMapMarker{Latitude: 35 + float64(i)*0.001, Longitude: -78 - float64(i)*0.001}
	}

	mapURL, err := staticMapURL("api-key", StaticMapRequest{Markers: markers})
	if err != nil {
		t.Fatal(err)
	}
	values := staticMapQuery(t, mapURL)
	parts := strings.Split(values.Get("markers"), "|")
	if len(parts) != staticMapMaxMarkers+1 {
		t.Fatalf("markers parts len = %d, want color plus %d markers", len(parts), staticMapMaxMarkers)
	}
	if len(mapURL) >= 16384 {
		t.Fatalf("map URL length = %d, want under Google Static Maps limit", len(mapURL))
	}
}

func TestStaticMapRedactsRequestErrors(t *testing.T) {
	client := &Client{
		apiKey: "secret-key",
		http: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, &url.Error{Op: "Get", URL: req.URL.String(), Err: errors.New("network down")}
		})},
	}

	_, err := client.StaticMap(context.Background(), StaticMapRequest{Markers: []StaticMapMarker{{Latitude: 35.7796, Longitude: -78.6382}}})
	if err == nil {
		t.Fatal("expected request error")
	}
	message := err.Error()
	for _, leaked := range []string{"secret-key", "key=", "maps.googleapis.com"} {
		if strings.Contains(message, leaked) {
			t.Fatalf("static map error leaked %q: %q", leaked, message)
		}
	}
	if !strings.Contains(message, "google static map request failed") {
		t.Fatalf("static map error = %q", message)
	}
}

func staticMapQuery(t *testing.T, mapURL string) url.Values {
	t.Helper()
	parsed, err := url.Parse(mapURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "https" || parsed.Host != "maps.googleapis.com" || parsed.Path != "/maps/api/staticmap" {
		t.Fatalf("unexpected static map endpoint: %s", mapURL)
	}
	return parsed.Query()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
