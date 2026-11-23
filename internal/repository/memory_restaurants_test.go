package repository

import (
	"context"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

func TestMemoryStoreVisitedRestaurantMapPointsDistinctVisitedRestaurants(t *testing.T) {
	store := NewMemoryStore()
	now := time.Date(2026, 5, 18, 20, 0, 0, 0, time.UTC)
	hanks := store.restaurants[0]
	patio := store.restaurants[1]
	noCoordinates := model.Restaurant{ID: uuid.New(), Name: "Mystery Counter"}
	store.visits = []model.Visit{
		demoVisit(hanks, &store.people[0], now.Add(-2*time.Hour), 2, "", nil, nil),
		demoVisit(patio, &store.people[1], now.Add(-1*time.Hour), 2, "", nil, nil),
		demoVisit(hanks, &store.people[2], now, 2, "", nil, nil),
		demoVisit(noCoordinates, &store.people[3], now.Add(time.Hour), 2, "", nil, nil),
	}

	points, err := store.VisitedRestaurantMapPoints(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 {
		t.Fatalf("points len = %d, want 2: %#v", len(points), points)
	}
	if points[0].RestaurantID != hanks.ID || points[0].VisitCount != 2 || !points[0].LatestVisitedAt.Equal(now) {
		t.Fatalf("first point = %#v, want deduped latest Hanks", points[0])
	}
	if points[0].Latitude != *hanks.Latitude || points[0].Longitude != *hanks.Longitude {
		t.Fatalf("first coordinates = %f/%f, want %f/%f", points[0].Latitude, points[0].Longitude, *hanks.Latitude, *hanks.Longitude)
	}
	if points[1].RestaurantID != patio.ID || points[1].VisitCount != 1 {
		t.Fatalf("second point = %#v, want Patio", points[1])
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

func TestMemoryStoreUpdateRestaurantGoogleMetadataRefreshesExistingValues(t *testing.T) {
	store := NewMemoryStore()
	restaurantID := store.restaurants[0].ID
	if err := store.UpdateRestaurant(context.Background(), restaurantID, model.RestaurantInput{
		Name:             "Hank's Corrected Diner",
		Address:          "202 Corrected Way",
		City:             "Garner",
		Phone:            "919-555-0199",
		Website:          "https://old.example",
		GoogleRating:     floatPtr(4.1),
		GooglePriceLevel: intPtr(1),
		Category:         "Southern",
	}); err != nil {
		t.Fatal(err)
	}

	lat := 35.8
	lng := -78.7
	rating := 4.9
	price := 3
	if err := store.UpdateRestaurantGoogleMetadata(context.Background(), restaurantID, model.GoogleRestaurantMetadata{
		Latitude:         &lat,
		Longitude:        &lng,
		Phone:            "919-555-0101",
		Website:          "https://fresh.example",
		GoogleRating:     &rating,
		GooglePriceLevel: &price,
	}); err != nil {
		t.Fatal(err)
	}

	restaurant, err := store.Restaurant(context.Background(), restaurantID)
	if err != nil {
		t.Fatal(err)
	}
	assertRestaurantGoogleMetadata(t, *restaurant, rating, price, lat, lng)
	if restaurant.Phone == nil || *restaurant.Phone != "919-555-0101" {
		t.Fatalf("Phone = %#v, want refreshed value", restaurant.Phone)
	}
	if restaurant.Website == nil || *restaurant.Website != "https://fresh.example" {
		t.Fatalf("Website = %#v, want refreshed value", restaurant.Website)
	}
	if restaurant.Category == nil || *restaurant.Category != "Southern" {
		t.Fatalf("Category = %#v, want existing value preserved when refresh category is empty", restaurant.Category)
	}
	visit := visitByID(t, store, store.visits[0].ID)
	if visit.Restaurant.GoogleRating == nil || *visit.Restaurant.GoogleRating != rating {
		t.Fatalf("visit restaurant copy was not refreshed: %#v", visit.Restaurant)
	}
}

func TestMemoryStoreUpdateRestaurantGoogleMetadataRefreshesCategory(t *testing.T) {
	store := NewMemoryStore()
	restaurantID := store.restaurants[0].ID
	if err := store.UpdateRestaurant(context.Background(), restaurantID, model.RestaurantInput{
		Name:     "Hank's Corrected Diner",
		Category: "Other",
	}); err != nil {
		t.Fatal(err)
	}

	if err := store.UpdateRestaurantGoogleMetadata(context.Background(), restaurantID, model.GoogleRestaurantMetadata{
		Category: "Korean",
	}); err != nil {
		t.Fatal(err)
	}

	restaurant, err := store.Restaurant(context.Background(), restaurantID)
	if err != nil {
		t.Fatal(err)
	}
	if restaurant.Category == nil || *restaurant.Category != "Korean" {
		t.Fatalf("Category = %#v, want refreshed Korean value", restaurant.Category)
	}
	visit := visitByID(t, store, store.visits[0].ID)
	if visit.Restaurant.Category == nil || *visit.Restaurant.Category != "Korean" {
		t.Fatalf("visit restaurant copy category was not refreshed: %#v", visit.Restaurant.Category)
	}
}
