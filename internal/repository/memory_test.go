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

func TestMemoryStoreVisitsNewestFirstUsesCreatedAtTieBreaker(t *testing.T) {
	store := NewMemoryStore()
	restaurant := store.restaurants[0]
	picker := store.people[0]
	visitedAt := time.Date(2026, 5, 15, 18, 0, 0, 0, time.UTC)
	olderCreated := demoVisit(restaurant, picker, visitedAt, 2, "older", nil, nil)
	olderCreated.CreatedAt = visitedAt.Add(time.Minute)
	newerCreated := demoVisit(restaurant, picker, visitedAt, 2, "newer", nil, nil)
	newerCreated.CreatedAt = visitedAt.Add(2 * time.Minute)
	store.visits = []model.Visit{olderCreated, newerCreated}

	visits, err := store.Visits(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if visits[0].ID != newerCreated.ID {
		t.Fatalf("expected newest created visit first, got %s", visits[0].ID)
	}
}

func TestMemoryStorePickerTurnNoVisitsStartsWithDaniel(t *testing.T) {
	store := NewMemoryStore()
	store.visits = nil

	turn, err := store.PickerTurn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if turn.LastPicker.Name != "" {
		t.Fatalf("expected no last picker, got %q", turn.LastPicker.Name)
	}
	if turn.NextPicker.Name != "Daniel" {
		t.Fatalf("expected Daniel next, got %q", turn.NextPicker.Name)
	}
}

func TestMemoryStorePickerTurnAdvancesRoundRobin(t *testing.T) {
	store := NewMemoryStore()
	store.visits = []model.Visit{
		demoVisit(store.restaurants[0], store.people[0], time.Date(2026, 5, 15, 18, 0, 0, 0, time.UTC), 2, "", nil, nil),
	}

	turn, err := store.PickerTurn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if turn.LastPicker.Name != "Daniel" {
		t.Fatalf("expected Daniel last, got %q", turn.LastPicker.Name)
	}
	if turn.NextPicker.Name != "Jen" {
		t.Fatalf("expected Jen next, got %q", turn.NextPicker.Name)
	}
}

func TestMemoryStorePickerTurnWrapsAfterAiden(t *testing.T) {
	store := NewMemoryStore()
	store.visits = []model.Visit{
		demoVisit(store.restaurants[0], store.people[3], time.Date(2026, 5, 15, 18, 0, 0, 0, time.UTC), 2, "", nil, nil),
	}

	turn, err := store.PickerTurn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if turn.LastPicker.Name != "Aiden" {
		t.Fatalf("expected Aiden last, got %q", turn.LastPicker.Name)
	}
	if turn.NextPicker.Name != "Daniel" {
		t.Fatalf("expected Daniel next, got %q", turn.NextPicker.Name)
	}
}

func TestMemoryStorePickerTurnUsesNewestVisitTime(t *testing.T) {
	store := NewMemoryStore()
	store.visits = []model.Visit{
		demoVisit(store.restaurants[0], store.people[3], time.Date(2026, 5, 1, 18, 0, 0, 0, time.UTC), 2, "", nil, nil),
		demoVisit(store.restaurants[0], store.people[1], time.Date(2026, 5, 15, 18, 0, 0, 0, time.UTC), 2, "", nil, nil),
	}

	turn, err := store.PickerTurn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if turn.LastPicker.Name != "Jen" {
		t.Fatalf("expected Jen last, got %q", turn.LastPicker.Name)
	}
	if turn.NextPicker.Name != "Caleb" {
		t.Fatalf("expected Caleb next, got %q", turn.NextPicker.Name)
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
