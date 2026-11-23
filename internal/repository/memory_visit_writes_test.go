package repository

import (
	"context"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/testutil"
	"github.com/google/uuid"
)

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
		PickerID:       &people[0].ID,
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

func TestMemoryStoreCreateVisitStoresPhotos(t *testing.T) {
	store := NewMemoryStore()
	people, err := store.People(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	input := model.VisitInput{
		RestaurantName: "Photo Counter",
		VisitedAt:      time.Now(),
		PickerID:       &people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
		Photos: []model.VisitPhotoInput{
			{DataURI: testVisitPhotoDataURI},
			{DataURI: testutil.PhotoURI(2)},
		},
	}

	visitID, err := store.CreateVisit(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}

	created := visitByID(t, store, *visitID)
	if len(created.Photos) != 2 {
		t.Fatalf("photos len = %d, want 2", len(created.Photos))
	}
	if created.Photos[0].DataURI != testVisitPhotoDataURI || created.Photos[0].ContentType != "image/jpeg" || created.Photos[0].ByteCount != len(testutil.JPEG(8, 6, 1)) || created.Photos[0].SortOrder != 0 {
		t.Fatalf("first photo = %#v", created.Photos[0])
	}
	if created.Photos[1].SortOrder != 1 {
		t.Fatalf("second photo sort order = %d, want 1", created.Photos[1].SortOrder)
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
		PickerID:       &people[0].ID,
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
		PickerID:       &people[0].ID,
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
		PickerID:   &people[0].ID,
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
		PickerID:     &store.people[2].ID,
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

func TestMemoryStoreUpdateVisitReconcilesPhotos(t *testing.T) {
	store := NewMemoryStore()
	visit := store.visits[0]
	visit.Photos = photosFromTestInput(t, visit.ID, []model.VisitPhotoInput{
		{DataURI: testVisitPhotoDataURI},
		{DataURI: testutil.PhotoURI(7)},
	}, 0)
	store.visits[0] = visit
	restaurantID := visit.Restaurant.ID

	input := model.VisitInput{
		RestaurantID: &restaurantID,
		VisitedAt:    visit.VisitedAt,
		PickerID:     &visit.Picker.ID,
		PriceLevel:   visit.PriceLevel,
		Ratings:      map[uuid.UUID]float64{store.people[0].ID: 9},
		KeepPhotoIDs: []uuid.UUID{visit.Photos[1].ID},
		Photos:       []model.VisitPhotoInput{{DataURI: testutil.PhotoURI(8)}},
	}

	if err := store.UpdateVisit(context.Background(), visit.ID, input); err != nil {
		t.Fatal(err)
	}

	updated := visitByID(t, store, visit.ID)
	if len(updated.Photos) != 2 {
		t.Fatalf("photos len = %d, want 2", len(updated.Photos))
	}
	if updated.Photos[0].ID != visit.Photos[1].ID || updated.Photos[0].SortOrder != 0 {
		t.Fatalf("kept photo = %#v, want original second photo first", updated.Photos[0])
	}
	if updated.Photos[1].ID == visit.Photos[0].ID || updated.Photos[1].DataURI != testutil.PhotoURI(8) || updated.Photos[1].SortOrder != 1 {
		t.Fatalf("new photo = %#v", updated.Photos[1])
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
		PickerID:   &people[0].ID,
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
