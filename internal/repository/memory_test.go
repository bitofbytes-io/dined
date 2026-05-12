package repository

import (
	"context"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

func TestMemoryStoreVisitsZeroReturnsAllVisits(t *testing.T) {
	store := NewMemoryStore()

	limited, err := store.Visits(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 {
		t.Fatalf("expected 2 limited visits, got %d", len(limited))
	}

	all, err := store.Visits(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != len(store.visits) {
		t.Fatalf("expected all visits, got %d of %d", len(all), len(store.visits))
	}
}

func TestMemoryStoreCreateVisitDoesNotReuseNameOnlyMatch(t *testing.T) {
	store := NewMemoryStore()
	people, err := store.People(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	input := model.VisitInput{
		RestaurantName: "Hank's Downtown Diner",
		Address:        "202 Other Street",
		VisitedAt:      time.Now(),
		PickerID:       people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	}

	visitID, err := store.CreateVisit(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}

	var created *model.Visit
	for i := range store.visits {
		if store.visits[i].ID == *visitID {
			created = &store.visits[i]
			break
		}
	}
	if created == nil {
		t.Fatal("created visit not found")
	}
	if created.Restaurant.Address == nil || *created.Restaurant.Address != "202 Other Street" {
		t.Fatalf("expected a distinct restaurant location, got %#v", created.Restaurant.Address)
	}
}

func TestMemoryStoreCreateVisitReusesNameAddressWithPlaceID(t *testing.T) {
	store := NewMemoryStore()
	people, err := store.People(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	input := model.VisitInput{
		RestaurantName: "Manual Cafe",
		Address:        "1 Test Way",
		VisitedAt:      time.Now(),
		PickerID:       people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	}

	firstID, err := store.CreateVisit(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	withPlace := input
	withPlace.VisitedAt = input.VisitedAt.Add(time.Hour)
	withPlace.GooglePlaceID = "manual-cafe-place"
	withPlace.Category = "Coffee"

	secondID, err := store.CreateVisit(context.Background(), withPlace)
	if err != nil {
		t.Fatal(err)
	}

	first := visitByID(t, store, *firstID)
	second := visitByID(t, store, *secondID)
	if first.Restaurant.ID != second.Restaurant.ID {
		t.Fatalf("expected place prefill to reuse restaurant %s, got %s", first.Restaurant.ID, second.Restaurant.ID)
	}
	if second.Restaurant.GooglePlaceID == nil || *second.Restaurant.GooglePlaceID != "manual-cafe-place" {
		t.Fatalf("expected Google Place ID to be attached, got %#v", second.Restaurant.GooglePlaceID)
	}
}

func visitByID(t *testing.T, store *MemoryStore, id uuid.UUID) model.Visit {
	t.Helper()
	for _, visit := range store.visits {
		if visit.ID == id {
			return visit
		}
	}
	t.Fatalf("visit %s not found", id)
	return model.Visit{}
}
