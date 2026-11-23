package repository

import (
	"context"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/testutil"
	"github.com/google/uuid"
)

var testVisitPhotoDataURI = testutil.PhotoURI(1)

// emptyMemoryStore is the memory store with its seeded people but no visits.
func emptyMemoryStore() *MemoryStore {
	store := NewMemoryStore()
	store.visits = nil
	store.restaurants = nil
	return store
}

func TestMemoryStoreRestaurantsByPlaceIDs(t *testing.T) {
	assertRestaurantsByPlaceIDs(t, emptyMemoryStore())
}

func TestMemoryStoreAllowsBackToBackPicks(t *testing.T) {
	assertBackToBackPicks(t, emptyMemoryStore())
}

func TestMemoryStoreEverybodyPicks(t *testing.T) {
	assertEverybodyPicks(t, emptyMemoryStore())
}

func TestMemoryStoreVisitFilters(t *testing.T) {
	assertVisitFilters(t, emptyMemoryStore())
}

func TestMemoryStoreNoVisitFilterOptionsWithoutVisits(t *testing.T) {
	assertNoVisitFilterOptionsWithoutVisits(t, emptyMemoryStore())
}

func TestMemoryStoreRejectsUnknownPicker(t *testing.T) {
	assertUnknownPickerRejected(t, emptyMemoryStore())
}

func TestMemoryStoreAggregatesNeedTwoRaters(t *testing.T) {
	assertAggregatesNeedTwoRaters(t, emptyMemoryStore())
}

func TestMemoryStoreNoAggregatesWithoutTwoRaters(t *testing.T) {
	assertNoAggregatesWithoutTwoRaters(t, emptyMemoryStore())
}

func TestMemoryStoreTrophyRecords(t *testing.T) {
	assertTrophyRecords(t, emptyMemoryStore())
}

func TestMemoryStoreYearInReview(t *testing.T) {
	assertYearInReview(t, emptyMemoryStore())
}

func TestMemoryStoreYearInReviewTiesBreakByRestaurantID(t *testing.T) {
	assertYearInReviewTiesBreakByRestaurantID(t, emptyMemoryStore())
}

func TestMemoryStoreYearInReviewWithoutVisits(t *testing.T) {
	assertYearInReviewWithoutVisits(t, emptyMemoryStore())
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

func TestMemoryStoreRejectsPlaceIDOwnedByAnotherRestaurant(t *testing.T) {
	assertChosenRestaurantPlaceOwnedElsewhereRejected(t, emptyMemoryStore())
}

func TestMemoryStoreRejectsDifferentPlaceForLinkedRestaurant(t *testing.T) {
	assertChosenRestaurantDifferentPlaceRejected(t, emptyMemoryStore())
}

func TestMemoryStoreAcceptsMatchingPlaceForChosenRestaurant(t *testing.T) {
	assertChosenRestaurantMatchingPlaceAccepted(t, emptyMemoryStore())
}

func TestMemoryStoreEditVisitIgnoresPlaceID(t *testing.T) {
	assertEditVisitIgnoresPlaceID(t, emptyMemoryStore())
}

func TestMemoryStoreIgnoresPlaceDetailsWithoutPlaceID(t *testing.T) {
	assertChosenRestaurantIgnoresDetailsWithoutPlaceID(t, emptyMemoryStore())
}

func photosFromTestInput(t *testing.T, visitID uuid.UUID, inputs []model.VisitPhotoInput, startOrder int) []model.VisitPhoto {
	t.Helper()
	_, photos, err := prepareVisitInput(context.Background(), model.VisitInput{RestaurantName: "fixture", VisitedAt: time.Now(), PriceLevel: 2, Ratings: map[uuid.UUID]float64{uuid.New(): 8}, Photos: inputs})
	if err != nil {
		t.Fatal(err)
	}
	return photosFromPrepared(visitID, photos, startOrder)
}
