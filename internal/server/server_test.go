package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
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
		PickerID:       people[0].ID,
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

	router := New(&config.Config{APIToken: "secret"}, store, places.NewClient("")).Router()
	req := httptest.NewRequest(http.MethodPost, "/restaurants/"+restaurants[0].ID.String()+"/delete", strings.NewReader("q=Amigos"))
	req.Header.Set("Authorization", "Bearer secret")
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

	router := New(&config.Config{APIToken: "secret"}, store, places.NewClient("")).Router()
	req := httptest.NewRequest(http.MethodPost, "/restaurants/"+restaurants[0].ID.String()+"/delete", strings.NewReader("q=Hank"))
	req.Header.Set("Authorization", "Bearer secret")
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
