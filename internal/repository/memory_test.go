package repository

import (
	"context"
	"math"
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
	withPlace.City = "Apex"

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
	if second.Restaurant.City == nil || *second.Restaurant.City != "Apex" {
		t.Fatalf("expected city to be attached, got %#v", second.Restaurant.City)
	}
}

func TestMemoryStoreStatsIncludesTrophyMetrics(t *testing.T) {
	store := NewMemoryStore()

	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.NewPlaces != 3 {
		t.Fatalf("NewPlaces = %d, want 3", stats.NewPlaces)
	}
	if stats.CitiesExplored != 3 {
		t.Fatalf("CitiesExplored = %d, want 3", stats.CitiesExplored)
	}
	if stats.BestPicker != "Jen" {
		t.Fatalf("BestPicker = %q, want Jen", stats.BestPicker)
	}
	if math.Abs(stats.BestPickerAverage-8.375) > 0.001 {
		t.Fatalf("BestPickerAverage = %f, want 8.375", stats.BestPickerAverage)
	}
	if len(stats.TopRestaurants) != 3 {
		t.Fatalf("expected 3 top restaurants, got %d", len(stats.TopRestaurants))
	}
	if stats.TopRestaurants[0].Name != "El Patio Verde" {
		t.Fatalf("top restaurant = %q, want El Patio Verde", stats.TopRestaurants[0].Name)
	}
	if math.Abs(stats.TopRestaurants[0].AverageRating-8.375) > 0.001 {
		t.Fatalf("top average = %f, want 8.375", stats.TopRestaurants[0].AverageRating)
	}
	if stats.TopRestaurants[0].RatingCount != 4 || stats.TopRestaurants[0].VisitCount != 1 {
		t.Fatalf("top counts = %d ratings/%d visits, want 4/1", stats.TopRestaurants[0].RatingCount, stats.TopRestaurants[0].VisitCount)
	}
}

func TestMemoryStoreTopRestaurantsRequireTwoRatingsAndSortDeterministically(t *testing.T) {
	store := NewMemoryStore()
	people := store.people
	now := time.Now()
	abacus := model.Restaurant{ID: uuid.New(), Name: "Abacus", City: strPtr("Apex")}
	alpha := model.Restaurant{ID: uuid.New(), Name: "Alpha", City: strPtr("Apex")}
	bravo := model.Restaurant{ID: uuid.New(), Name: "Bravo", City: strPtr("Cary")}
	solo := model.Restaurant{ID: uuid.New(), Name: "Solo", City: strPtr("Raleigh")}
	store.restaurants = []model.Restaurant{abacus, alpha, bravo, solo}
	store.visits = []model.Visit{
		demoVisit(alpha, people[0], now, 2, "", []model.Rating{{Person: people[0], Score: 9}, {Person: people[1], Score: 8}}, nil),
		demoVisit(bravo, people[1], now, 2, "", []model.Rating{{Person: people[0], Score: 8.5}, {Person: people[1], Score: 8.5}, {Person: people[2], Score: 8.5}}, nil),
		demoVisit(abacus, people[2], now, 2, "", []model.Rating{{Person: people[0], Score: 8.5}, {Person: people[1], Score: 8.5}}, nil),
		demoVisit(solo, people[3], now, 2, "", []model.Rating{{Person: people[0], Score: 10}}, nil),
	}

	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Bravo", "Abacus", "Alpha"}
	if len(stats.TopRestaurants) != len(want) {
		t.Fatalf("got %d top restaurants, want %d: %#v", len(stats.TopRestaurants), len(want), stats.TopRestaurants)
	}
	for i, name := range want {
		if stats.TopRestaurants[i].Name != name {
			t.Fatalf("top restaurant %d = %q, want %q", i, stats.TopRestaurants[i].Name, name)
		}
	}
	if stats.CitiesExplored != 3 {
		t.Fatalf("CitiesExplored = %d, want 3", stats.CitiesExplored)
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
