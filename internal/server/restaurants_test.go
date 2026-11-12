package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/google/uuid"
)

func TestRouterDeletesUnvisitedRestaurantAndPreservesSearch(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Amigos",
		VisitedAt:      time.Now(),
		PickerID:       &people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	restaurants, err := store.Restaurants(ctx, "Amigos")
	if err != nil {
		t.Fatal(err)
	}
	if len(restaurants) != 1 {
		t.Fatalf("expected one Amigos restaurant, got %d", len(restaurants))
	}
	if err := store.DeleteVisit(ctx, *visitID); err != nil {
		t.Fatal(err)
	}

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/restaurants/"+restaurants[0].ID.String()+"/delete", strings.NewReader("q=Amigos"))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "/search?q=Amigos" {
		t.Fatalf("redirect location = %q, want /search?q=Amigos", location)
	}
	remaining, err := store.Restaurants(ctx, "Amigos")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected Amigos to be deleted, got %#v", remaining)
	}
}

func TestRouterRefusesVisitedRestaurantDelete(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	restaurants, err := store.Restaurants(ctx, "Hank")
	if err != nil {
		t.Fatal(err)
	}
	if len(restaurants) != 1 {
		t.Fatalf("expected one Hank restaurant, got %d", len(restaurants))
	}

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/restaurants/"+restaurants[0].ID.String()+"/delete", strings.NewReader("q=Hank"))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusConflict)
	}
	remaining, err := store.Restaurants(ctx, "Hank")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected visited restaurant to remain, got %#v", remaining)
	}
}

func TestRouterUpdateRestaurantReturnsToEditDine(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Return Flow Diner",
		VisitedAt:      time.Now(),
		PickerID:       &people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	visit, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	if visit == nil {
		t.Fatal("expected created visit")
	}

	form := url.Values{}
	form.Set("restaurant_name", "Return Flow Diner Updated")
	form.Set("return_visit_id", visitID.String())

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/restaurants/"+visit.Restaurant.ID.String(), strings.NewReader(form.Encode()))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	wantLocation := "/visits/" + visitID.String() + "/edit"
	if location := rec.Header().Get("Location"); location != wantLocation {
		t.Fatalf("redirect location = %q, want %q", location, wantLocation)
	}
	updated, err := store.Restaurant(ctx, visit.Restaurant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated == nil || updated.Name != "Return Flow Diner Updated" {
		t.Fatalf("restaurant was not updated: %#v", updated)
	}
}

func TestRouterUpdateRestaurantIgnoresMismatchedReturnVisitID(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	firstVisitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "First Diner",
		VisitedAt:      time.Now(),
		PickerID:       &people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondVisitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Second Diner",
		VisitedAt:      time.Now(),
		PickerID:       &people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 7},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstVisit, err := store.Visit(ctx, *firstVisitID)
	if err != nil {
		t.Fatal(err)
	}
	if firstVisit == nil {
		t.Fatal("expected first visit")
	}

	form := url.Values{}
	form.Set("restaurant_name", "First Diner Updated")
	form.Set("return_visit_id", secondVisitID.String())

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/restaurants/"+firstVisit.Restaurant.ID.String(), strings.NewReader(form.Encode()))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	wantLocation := "/restaurants/" + firstVisit.Restaurant.ID.String()
	if location := rec.Header().Get("Location"); location != wantLocation {
		t.Fatalf("redirect location = %q, want %q", location, wantLocation)
	}
}

func TestRouterGoogleRefreshRedirectsToEditWithReturnVisitIDWhenUnconfigured(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Google Return Diner",
		GooglePlaceID:  "place-1",
		VisitedAt:      time.Now(),
		PickerID:       &people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	visit, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	if visit == nil {
		t.Fatal("expected created visit")
	}

	form := url.Values{}
	form.Set("return_visit_id", visitID.String())

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/restaurants/"+visit.Restaurant.ID.String()+"/google-refresh", strings.NewReader(form.Encode()))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	wantLocation := "/restaurants/" + visit.Restaurant.ID.String() + "/edit?google_refresh=unconfigured&return_visit_id=" + visitID.String()
	if location := rec.Header().Get("Location"); location != wantLocation {
		t.Fatalf("redirect location = %q, want %q", location, wantLocation)
	}
}
