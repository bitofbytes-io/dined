package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/go-chi/chi/v5"
)

func TestNearbyLeavesOutPlacesMarkedClosed(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	for _, input := range []model.WantToTryInput{
		{RestaurantName: "Shuttered Cafe", GooglePlaceID: "place-shuttered"},
		{RestaurantName: "Open Cafe", GooglePlaceID: "place-open"},
	} {
		if _, err := store.SaveWantToTry(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := store.RestaurantsByPlaceIDs(ctx, []string{"place-shuttered"})
	if err != nil || len(saved) != 1 {
		t.Fatalf("saved = %#v, err = %v", saved, err)
	}
	shuttered := saved["place-shuttered"]
	if err := store.UpdateRestaurant(ctx, shuttered.ID, model.RestaurantInput{Name: shuttered.Name, Closed: true}); err != nil {
		t.Fatal(err)
	}
	fakePlaces := &fakeGooglePlacesClient{
		configured: true,
		nearbyPlaces: []places.Place{
			{ID: "place-shuttered", DisplayName: places.Name{Text: "Shuttered Cafe"}},
			{ID: "place-open", DisplayName: places.Name{Text: "Open Cafe"}},
			// Google calls this one closed, but nobody has marked it, so it stays.
			{ID: "place-unmarked", DisplayName: places.Name{Text: "Unmarked Closed Grill"}, BusinessStatus: places.BusinessStatusClosedPermanently},
		},
	}
	rec := httptest.NewRecorder()

	New(nil, store, fakePlaces, nil, nil).Nearby(rec, httptest.NewRequestWithContext(ctx, http.MethodGet, "/nearby?lat=35.7796&lng=-78.6382", nil))

	body := rec.Body.String()
	if rec.Code != http.StatusOK || strings.Contains(body, "Shuttered Cafe") {
		t.Fatalf("nearby = %d, should leave out the closed Shuttered Cafe:\n%s", rec.Code, body)
	}
	for _, name := range []string{"Open Cafe", "Unmarked Closed Grill"} {
		if !strings.Contains(body, name) {
			t.Fatalf("nearby missing %q:\n%s", name, body)
		}
	}
}

func TestRefreshRestaurantGoogleOnlySuggestsClosure(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	restaurants, err := store.Restaurants(ctx, "Hank")
	if err != nil || len(restaurants) != 1 {
		t.Fatalf("restaurants = %#v, err = %v", restaurants, err)
	}
	hanks := restaurants[0]
	fakePlaces := &fakeGooglePlacesClient{
		configured:   true,
		detailsPlace: &places.Place{ID: "demo-hanks", BusinessStatus: places.BusinessStatusClosedPermanently},
	}
	handler := New(nil, store, fakePlaces, nil, nil)
	refresh := func() *httptest.ResponseRecorder {
		req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/restaurants/"+hanks.ID.String()+"/google-refresh", nil)
		routeContext := chi.NewRouteContext()
		routeContext.URLParams.Add("id", hanks.ID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
		rec := httptest.NewRecorder()
		handler.RefreshRestaurantGoogle(rec, req)
		return rec
	}

	rec := refresh()
	if got := rec.Header().Get("Location"); rec.Code != http.StatusSeeOther || !strings.Contains(got, "google_refresh=closed") {
		t.Fatalf("refresh = %d to %q, want a closed suggestion", rec.Code, got)
	}
	after, err := store.Restaurant(ctx, hanks.ID)
	if err != nil || after == nil || after.IsClosed() {
		t.Fatalf("restaurant after refresh = %#v, err = %v, want still open", after, err)
	}

	// Once someone has marked it closed there is nothing left to suggest.
	if err := store.UpdateRestaurant(ctx, hanks.ID, model.RestaurantInput{Name: hanks.Name, Closed: true}); err != nil {
		t.Fatal(err)
	}
	if got := refresh().Header().Get("Location"); !strings.Contains(got, "google_refresh=updated") {
		t.Fatalf("refresh of a closed restaurant redirected to %q, want updated", got)
	}
}

func TestRestaurantInputParsesClosed(t *testing.T) {
	parse := func(form url.Values) model.RestaurantInput {
		t.Helper()
		form.Set("restaurant_name", "Hank's")
		req := httptest.NewRequest(http.MethodPost, "/restaurants/id", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		input, err := (&Handler{}).restaurantInput(req)
		if err != nil {
			t.Fatal(err)
		}
		return input
	}
	if !parse(url.Values{"closed": {"true"}}).Closed {
		t.Fatal("closed=true should mark the restaurant closed")
	}
	if parse(url.Values{}).Closed {
		t.Fatal("an unchecked box should leave the restaurant open")
	}
}

func TestGoogleRefreshNoticeSuggestsClosure(t *testing.T) {
	if got := googleRefreshNotice("closed"); !strings.Contains(got, "permanently closed") {
		t.Fatalf("notice = %q", got)
	}
}
