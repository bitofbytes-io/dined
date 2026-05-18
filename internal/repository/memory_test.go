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
	rating := 4.3
	price := 2
	lat := 35.7903375
	lng := -78.6631725
	withPlace.GoogleMetadata = model.GoogleRestaurantMetadata{
		Latitude:         &lat,
		Longitude:        &lng,
		Phone:            "(919) 723-9353",
		Website:          "https://tupelohoneycafe.com/restaurant/raleigh/",
		GoogleRating:     &rating,
		GooglePriceLevel: &price,
	}

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
	assertRestaurantGoogleMetadata(t, second.Restaurant, rating, price, lat, lng)
}

func TestMemoryStoreCreateVisitUpdatesExistingRestaurantIDGoogleMetadata(t *testing.T) {
	store := NewMemoryStore()
	people, err := store.People(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	input := model.VisitInput{
		RestaurantName: "Manual Cafe",
		Address:        "1 Test Way",
		GooglePlaceID:  "manual-cafe-place",
		Category:       "Coffee",
		VisitedAt:      time.Now(),
		PickerID:       people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	}
	firstID, err := store.CreateVisit(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	first := visitByID(t, store, *firstID)
	if first.Restaurant.GoogleRating != nil {
		t.Fatalf("expected first visit to have no Google rating, got %#v", first.Restaurant.GoogleRating)
	}

	rating := 4.6
	price := 3
	lat := 35.7
	lng := -78.6
	withMetadata := input
	withMetadata.RestaurantID = &first.Restaurant.ID
	withMetadata.VisitedAt = input.VisitedAt.Add(time.Hour)
	withMetadata.Category = "Other"
	withMetadata.GoogleMetadata = model.GoogleRestaurantMetadata{
		Latitude:         &lat,
		Longitude:        &lng,
		Phone:            "919-555-1212",
		Website:          "https://example.com",
		GoogleRating:     &rating,
		GooglePriceLevel: &price,
	}

	secondID, err := store.CreateVisit(context.Background(), withMetadata)
	if err != nil {
		t.Fatal(err)
	}
	second := visitByID(t, store, *secondID)
	assertRestaurantGoogleMetadata(t, second.Restaurant, rating, price, lat, lng)
	if second.Restaurant.Category == nil || *second.Restaurant.Category != "Coffee" {
		t.Fatalf("expected existing category to be preserved, got %#v", second.Restaurant.Category)
	}
}

func TestMemoryStoreDeleteRestaurantIfUnvisitedDeletesOrphan(t *testing.T) {
	store := NewMemoryStore()
	restaurant := model.Restaurant{ID: uuid.New(), Name: "Amigos"}
	store.restaurants = append(store.restaurants, restaurant)

	deleted, err := store.DeleteRestaurantIfUnvisited(context.Background(), restaurant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected orphan restaurant to be deleted")
	}
	if got, err := store.Restaurant(context.Background(), restaurant.ID); err != nil {
		t.Fatal(err)
	} else if got != nil {
		t.Fatalf("expected restaurant to be gone, got %#v", got)
	}
}

func TestMemoryStoreDeleteRestaurantIfUnvisitedRefusesVisitedRestaurant(t *testing.T) {
	store := NewMemoryStore()
	restaurant := store.restaurants[0]

	deleted, err := store.DeleteRestaurantIfUnvisited(context.Background(), restaurant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("expected visited restaurant deletion to be refused")
	}
	if got, err := store.Restaurant(context.Background(), restaurant.ID); err != nil {
		t.Fatal(err)
	} else if got == nil {
		t.Fatal("expected visited restaurant to remain")
	}
}

func TestMemoryStoreCreateVisitStoresGoogleMetadata(t *testing.T) {
	store := NewMemoryStore()
	people, err := store.People(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lat := 35.7796
	lng := -78.6382
	rating := 4.7
	price := 3
	input := model.VisitInput{
		RestaurantName: "Google Grill",
		Address:        "44 Search Street",
		City:           "Raleigh",
		GooglePlaceID:  "google-grill-place",
		GoogleMetadata: model.GoogleRestaurantMetadata{
			Latitude:         &lat,
			Longitude:        &lng,
			Phone:            "919-555-0100",
			Website:          "https://example.com",
			GoogleRating:     &rating,
			GooglePriceLevel: &price,
		},
		Category:   "American",
		VisitedAt:  time.Now(),
		PickerID:   people[0].ID,
		PriceLevel: 2,
		Ratings:    map[uuid.UUID]float64{people[0].ID: 8},
	}

	visitID, err := store.CreateVisit(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}

	visit := visitByID(t, store, *visitID)
	if visit.Restaurant.Phone == nil || *visit.Restaurant.Phone != "919-555-0100" {
		t.Fatalf("phone = %#v, want Google metadata", visit.Restaurant.Phone)
	}
	if visit.Restaurant.Website == nil || *visit.Restaurant.Website != "https://example.com" {
		t.Fatalf("website = %#v, want Google metadata", visit.Restaurant.Website)
	}
	if visit.Restaurant.GoogleRating == nil || *visit.Restaurant.GoogleRating != 4.7 {
		t.Fatalf("google rating = %#v, want 4.7", visit.Restaurant.GoogleRating)
	}
	if visit.Restaurant.GooglePriceLevel == nil || *visit.Restaurant.GooglePriceLevel != 3 {
		t.Fatalf("google price = %#v, want 3", visit.Restaurant.GooglePriceLevel)
	}
	if visit.Restaurant.Latitude == nil || *visit.Restaurant.Latitude != lat || visit.Restaurant.Longitude == nil || *visit.Restaurant.Longitude != lng {
		t.Fatalf("coordinates = %#v/%#v, want Google metadata", visit.Restaurant.Latitude, visit.Restaurant.Longitude)
	}
}

func TestMemoryStoreUpdateVisitReplacesRatingsTagsAndNotes(t *testing.T) {
	store := NewMemoryStore()
	visit := store.visits[0]
	restaurantID := visit.Restaurant.ID
	visitedAt := time.Date(2026, 5, 16, 19, 30, 0, 0, time.UTC)
	input := model.VisitInput{
		RestaurantID: &restaurantID,
		VisitedAt:    visitedAt,
		PickerID:     store.people[2].ID,
		PriceLevel:   4,
		Notes:        "Updated after the table talked it through.",
		Ratings: map[uuid.UUID]float64{
			store.people[0].ID: 9,
			store.people[1].ID: 7.5,
		},
		TagIDs: []uuid.UUID{store.tags[2].ID},
	}

	if err := store.UpdateVisit(context.Background(), visit.ID, input); err != nil {
		t.Fatal(err)
	}

	updated := visitByID(t, store, visit.ID)
	if !updated.VisitedAt.Equal(visitedAt) || updated.Picker.ID != store.people[2].ID || updated.PriceLevel != 4 {
		t.Fatalf("visit fields were not updated: %#v", updated)
	}
	if updated.Notes == nil || *updated.Notes != input.Notes {
		t.Fatalf("notes = %#v, want updated notes", updated.Notes)
	}
	if len(updated.Ratings) != 2 {
		t.Fatalf("ratings len = %d, want 2", len(updated.Ratings))
	}
	if updated.Ratings[0].Person.Name != "Daniel" || updated.Ratings[1].Person.Name != "Jen" {
		t.Fatalf("ratings order = %#v, want family sort order", updated.Ratings)
	}
	if len(updated.Tags) != 1 || updated.Tags[0].Name != "Great Service" {
		t.Fatalf("tags = %#v, want Great Service", updated.Tags)
	}
}

func TestMemoryStoreUpdateRestaurantSyncsVisits(t *testing.T) {
	store := NewMemoryStore()
	restaurant := store.restaurants[0]
	rating := 4.9
	price := 2
	input := model.RestaurantInput{
		Name:             "Hank's Corrected Diner",
		Address:          "202 Corrected Way",
		City:             "Garner",
		Phone:            "919-555-0199",
		Website:          "https://hanks.example",
		GoogleRating:     &rating,
		GooglePriceLevel: &price,
		Category:         "Southern",
		IsChain:          true,
	}

	if err := store.UpdateRestaurant(context.Background(), restaurant.ID, input); err != nil {
		t.Fatal(err)
	}

	updated, err := store.Restaurant(context.Background(), restaurant.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != input.Name || updated.Address == nil || *updated.Address != input.Address || !updated.IsChain {
		t.Fatalf("restaurant was not updated: %#v", updated)
	}
	visit := visitByID(t, store, store.visits[0].ID)
	if visit.Restaurant.Name != input.Name || visit.Restaurant.Phone == nil || *visit.Restaurant.Phone != input.Phone {
		t.Fatalf("visit restaurant copy was not synced: %#v", visit.Restaurant)
	}
}

func TestMemoryStoreCreateVisitPreservesRestaurantOverrides(t *testing.T) {
	store := NewMemoryStore()
	people := store.people
	restaurantID := store.restaurants[0].ID
	rating := 4.8
	if err := store.UpdateRestaurant(context.Background(), restaurantID, model.RestaurantInput{
		Name:         "Hank's Corrected Diner",
		Address:      "202 Corrected Way",
		City:         "Garner",
		Phone:        "919-555-0199",
		GoogleRating: &rating,
		Category:     "Southern",
	}); err != nil {
		t.Fatal(err)
	}

	input := model.VisitInput{
		RestaurantName: "Google's Hank Name",
		Address:        "101 Google Way",
		City:           "Raleigh",
		GooglePlaceID:  "demo-hanks",
		GoogleMetadata: model.GoogleRestaurantMetadata{
			Phone:        "919-000-0000",
			GoogleRating: floatPtr(3.2),
		},
		Category:   "American",
		VisitedAt:  time.Now(),
		PickerID:   people[0].ID,
		PriceLevel: 2,
		Ratings:    map[uuid.UUID]float64{people[0].ID: 8},
	}
	if _, err := store.CreateVisit(context.Background(), input); err != nil {
		t.Fatal(err)
	}

	restaurant, err := store.Restaurant(context.Background(), restaurantID)
	if err != nil {
		t.Fatal(err)
	}
	if restaurant.Name != "Hank's Corrected Diner" ||
		restaurant.Address == nil || *restaurant.Address != "202 Corrected Way" ||
		restaurant.Phone == nil || *restaurant.Phone != "919-555-0199" ||
		restaurant.Category == nil || *restaurant.Category != "Southern" ||
		restaurant.GoogleRating == nil || *restaurant.GoogleRating != 4.8 {
		t.Fatalf("restaurant override was overwritten: %#v", restaurant)
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

func assertRestaurantGoogleMetadata(t *testing.T, restaurant model.Restaurant, rating float64, price int, lat float64, lng float64) {
	t.Helper()
	if restaurant.Latitude == nil || *restaurant.Latitude != lat {
		t.Fatalf("Latitude = %#v, want %f", restaurant.Latitude, lat)
	}
	if restaurant.Longitude == nil || *restaurant.Longitude != lng {
		t.Fatalf("Longitude = %#v, want %f", restaurant.Longitude, lng)
	}
	if restaurant.Phone == nil || *restaurant.Phone == "" {
		t.Fatalf("Phone = %#v", restaurant.Phone)
	}
	if restaurant.Website == nil || *restaurant.Website == "" {
		t.Fatalf("Website = %#v", restaurant.Website)
	}
	if restaurant.GoogleRating == nil || *restaurant.GoogleRating != rating {
		t.Fatalf("GoogleRating = %#v, want %f", restaurant.GoogleRating, rating)
	}
	if restaurant.GooglePriceLevel == nil || *restaurant.GooglePriceLevel != price {
		t.Fatalf("GooglePriceLevel = %#v, want %d", restaurant.GooglePriceLevel, price)
	}
}
