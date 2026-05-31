package handler

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/go-chi/chi/v5"
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

func TestSearchRejectsOverlongQuery(t *testing.T) {
	handler := New(nil, nil, nil, nil, nil)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/search?q="+strings.Repeat("a", maxPlacesQueryLength+1), nil)
	rec := httptest.NewRecorder()

	handler.Search(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestNearbyRejectsOverlongNearQuery(t *testing.T) {
	handler := New(nil, nil, nil, nil, nil)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/nearby?near="+strings.Repeat("a", maxPlacesQueryLength+1), nil)
	rec := httptest.NewRecorder()

	handler.Nearby(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestNearbyRejectsInvalidCoordinates(t *testing.T) {
	handler := New(nil, nil, nil, nil, nil)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/nearby?lat=91&lng=-78.638200", nil)
	rec := httptest.NewRecorder()

	handler.Nearby(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
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

func TestTrophyMapUsesStoredCoordinatesOnly(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	expected, err := store.VisitedRestaurantMapPoints(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(expected) == 0 {
		t.Fatal("expected memory store to have demo map points")
	}
	fakePlaces := &fakeGooglePlacesClient{
		configured: true,
		staticMapImage: &places.StaticMapImage{
			Data:        []byte("png"),
			ContentType: "image/png",
		},
	}
	handler := New(nil, store, fakePlaces, nil, nil)
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/trophy-case/map.png?latitude=1&longitude=2", nil)
	rec := httptest.NewRecorder()

	handler.TrophyMap(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if rec.Body.String() != "png" {
		t.Fatalf("body = %q, want static map image bytes", rec.Body.String())
	}
	if rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("content type = %q", rec.Header().Get("Content-Type"))
	}
	if fakePlaces.staticMapCalls != 1 {
		t.Fatalf("static map calls = %d, want 1", fakePlaces.staticMapCalls)
	}
	if len(fakePlaces.staticMapRequest.Markers) != len(expected) {
		t.Fatalf("markers len = %d, want %d", len(fakePlaces.staticMapRequest.Markers), len(expected))
	}
	for i, point := range expected {
		marker := fakePlaces.staticMapRequest.Markers[i]
		if marker.Latitude != point.Latitude || marker.Longitude != point.Longitude {
			t.Fatalf("marker %d = %#v, want coordinates from %#v", i, marker, point)
		}
		if marker.Latitude == 1 || marker.Longitude == 2 {
			t.Fatalf("marker %d used request query coordinates: %#v", i, marker)
		}
	}

	second := httptest.NewRecorder()
	handler.TrophyMap(second, httptest.NewRequestWithContext(ctx, http.MethodGet, "/trophy-case/map.png", nil))
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, want %d: %s", second.Code, http.StatusOK, second.Body.String())
	}
	if second.Body.String() != "png" {
		t.Fatalf("second body = %q, want cached static map image bytes", second.Body.String())
	}
	if fakePlaces.staticMapCalls != 1 {
		t.Fatalf("static map calls after cache hit = %d, want 1", fakePlaces.staticMapCalls)
	}
}

func TestTrophyMapRequestUsesDeterministicViewport(t *testing.T) {
	points := []model.RestaurantMapPoint{
		{RestaurantID: uuid.New(), Name: "First", Latitude: 35.7796, Longitude: -78.6382},
		{RestaurantID: uuid.New(), Name: "Second", Latitude: 35.7327, Longitude: -78.8503},
	}

	request := staticMapRequest(points)

	if request.Viewport == nil {
		t.Fatal("expected deterministic static map viewport")
	}
	if request.Viewport.Zoom != 11 {
		t.Fatalf("zoom = %d, want 11", request.Viewport.Zoom)
	}
	if math.Abs(request.Viewport.Latitude-35.756153) > 0.000001 || math.Abs(request.Viewport.Longitude-(-78.74425)) > 0.000001 {
		t.Fatalf("viewport = %#v", request.Viewport)
	}
}

func TestTrophyMapLabelsTruncateAndCollapseWhitespace(t *testing.T) {
	points := []model.RestaurantMapPoint{
		{RestaurantID: uuid.New(), Name: "  Hank's   Downtown  Diner  ", Latitude: 35.7796, Longitude: -78.6382},
		{RestaurantID: uuid.New(), Name: "Short Name", Latitude: 35.7327, Longitude: -78.8503},
	}

	labels := trophyMapLabels(points)

	if len(labels) != 2 {
		t.Fatalf("labels len = %d, want 2", len(labels))
	}
	if labels[0].Name != "Hank's Downto..." {
		t.Fatalf("long label = %q", labels[0].Name)
	}
	if labels[1].Name != "Short Name" {
		t.Fatalf("short label = %q", labels[1].Name)
	}
	for _, label := range labels {
		if !strings.HasSuffix(label.Left, "%") || !strings.HasSuffix(label.Top, "%") {
			t.Fatalf("label missing percent positions: %#v", label)
		}
	}
}

func TestRefreshRestaurantGoogleUpdatesMappedCategory(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	restaurants, err := store.Restaurants(ctx, "Hank")
	if err != nil {
		t.Fatal(err)
	}
	if len(restaurants) != 1 {
		t.Fatalf("restaurants len = %d, want 1", len(restaurants))
	}
	restaurantID := restaurants[0].ID
	fakePlaces := &fakeGooglePlacesClient{
		configured: true,
		detailsPlace: &places.Place{
			ID:    "demo-hanks",
			Types: []string{"korean_restaurant", "restaurant"},
		},
	}
	handler := New(nil, store, fakePlaces, nil, nil)
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/restaurants/"+restaurantID.String()+"/google-refresh", nil)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("id", restaurantID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
	rec := httptest.NewRecorder()

	handler.RefreshRestaurantGoogle(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	restaurant, err := store.Restaurant(ctx, restaurantID)
	if err != nil {
		t.Fatal(err)
	}
	if restaurant.Category == nil || *restaurant.Category != "Korean" {
		t.Fatalf("Category = %#v, want Korean", restaurant.Category)
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

func TestVisitInputRejectsOutOfRangeCoordinates(t *testing.T) {
	personID := uuid.New()
	form := url.Values{
		"restaurant_name":             {"Far Away Grill"},
		"latitude":                    {"35.779600"},
		"longitude":                   {"181"},
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

	_, err := (&Handler{}).visitInput(req)
	if err == nil || !strings.Contains(err.Error(), "longitude must be between -180 and 180") {
		t.Fatalf("error = %v, want longitude range error", err)
	}
}

func TestVisitInputParsesPhotos(t *testing.T) {
	personID := uuid.New()
	keptPhotoID := uuid.New()
	form := url.Values{
		"restaurant_name":             {"Photo Diner"},
		"visited_at":                  {apptime.FormatDatetimeLocal(time.Now())},
		"picker_id":                   {personID.String()},
		"price_level":                 {"2"},
		"rating_" + personID.String(): {"8"},
		"keep_photo_id":               {keptPhotoID.String()},
		"photo_data_uri": {
			"data:image/jpeg;base64,aGVsbG8=",
			"data:image/jpeg;base64,dGFjbw==",
		},
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
	if len(input.KeepPhotoIDs) != 1 || input.KeepPhotoIDs[0] != keptPhotoID {
		t.Fatalf("kept photo ids = %#v", input.KeepPhotoIDs)
	}
	if len(input.Photos) != 2 || input.Photos[0].DataURI != "data:image/jpeg;base64,aGVsbG8=" || input.Photos[1].DataURI != "data:image/jpeg;base64,dGFjbw==" {
		t.Fatalf("photos = %#v", input.Photos)
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

type fakeGooglePlacesClient struct {
	configured       bool
	detailsPlace     *places.Place
	staticMapImage   *places.StaticMapImage
	staticMapErr     error
	staticMapRequest places.StaticMapRequest
	staticMapCalls   int
}

func (f *fakeGooglePlacesClient) Configured() bool {
	return f.configured
}

func (f *fakeGooglePlacesClient) TextSearch(context.Context, string) ([]places.Place, error) {
	return nil, nil
}

func (f *fakeGooglePlacesClient) TextSearchNear(context.Context, string, float64, float64) ([]places.Place, error) {
	return nil, nil
}

func (f *fakeGooglePlacesClient) Nearby(context.Context, float64, float64) ([]places.Place, error) {
	return nil, nil
}

func (f *fakeGooglePlacesClient) Details(_ context.Context, _ string) (*places.Place, error) {
	return f.detailsPlace, nil
}

func (f *fakeGooglePlacesClient) StaticMap(_ context.Context, request places.StaticMapRequest) (*places.StaticMapImage, error) {
	f.staticMapCalls++
	f.staticMapRequest = places.StaticMapRequest{
		Markers:  append([]places.StaticMapMarker(nil), request.Markers...),
		Viewport: request.Viewport,
	}
	if f.staticMapErr != nil {
		return nil, f.staticMapErr
	}
	if f.staticMapImage != nil {
		return f.staticMapImage, nil
	}
	return &places.StaticMapImage{Data: []byte("png"), ContentType: "image/png"}, nil
}
