package repository

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

// These checks run against both stores. A visit that chooses an existing
// restaurant must never move another restaurant's Google place ID or Places
// metadata onto it. Each expects a store with the seeded people and no visits.

// placeTestInput is a visit at name with one rating.
func placeTestInput(t *testing.T, store DinerStore, name string) model.VisitInput {
	t.Helper()
	daniel, _, _, _ := storePeople(t, store)
	return model.VisitInput{
		RestaurantName: name,
		VisitedAt:      time.Date(2026, 9, 20, 18, 0, 0, 0, time.UTC),
		PickerID:       &daniel.ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{daniel.ID: 8},
	}
}

// googleOwnedMetadata is Places metadata for the place a restaurant owns.
func googleOwnedMetadata() model.GoogleRestaurantMetadata {
	latitude, longitude, rating, priceLevel := 35.91, -79.05, 4.7, 3
	return model.GoogleRestaurantMetadata{
		Latitude: &latitude, Longitude: &longitude, Phone: "919-555-0199",
		Website: "https://owner.example", GoogleRating: &rating, GooglePriceLevel: &priceLevel,
	}
}

// createPlaceTestRestaurant logs a first visit to create a restaurant and returns it.
func createPlaceTestRestaurant(t *testing.T, store DinerStore, input model.VisitInput) model.Restaurant {
	t.Helper()
	id, err := store.CreateVisit(context.Background(), input)
	if err != nil {
		t.Fatalf("create %s: %v", input.RestaurantName, err)
	}
	return storeRestaurant(t, store, *id)
}

func storeRestaurant(t *testing.T, store DinerStore, visitID uuid.UUID) model.Restaurant {
	t.Helper()
	visit, err := store.Visit(context.Background(), visitID)
	if err != nil {
		t.Fatal(err)
	}
	if visit == nil {
		t.Fatalf("visit %s not found", visitID)
	}
	return visit.Restaurant
}

func storeRestaurantByID(t *testing.T, store DinerStore, id uuid.UUID) model.Restaurant {
	t.Helper()
	restaurant, err := store.Restaurant(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if restaurant == nil {
		t.Fatalf("restaurant %s not found", id)
	}
	return *restaurant
}

func storeVisitCount(t *testing.T, store DinerStore) int {
	t.Helper()
	visits, err := store.VisitsPage(context.Background(), 1000, 0)
	if err != nil {
		t.Fatal(err)
	}
	return len(visits)
}

// restaurantPlaceFields are the fields a visit can fill on a chosen restaurant.
func restaurantPlaceFields(r model.Restaurant) []any {
	return []any{r.Name, r.Address, r.City, r.GooglePlaceID, r.Latitude, r.Longitude, r.Phone, r.Website, r.GoogleRating, r.GooglePriceLevel, r.Category}
}

func assertRestaurantUnchanged(t *testing.T, label string, before, after model.Restaurant) {
	t.Helper()
	if !reflect.DeepEqual(restaurantPlaceFields(before), restaurantPlaceFields(after)) {
		t.Fatalf("%s changed:\nbefore %#v\nafter  %#v", label, before, after)
	}
}

// assertChosenRestaurantPlaceOwnedElsewhereRejected chooses restaurant A but
// submits the place ID (and the Places metadata) of restaurant B. The visit is
// rejected with a conflict naming B, and neither restaurant changes: A must
// not gain B's place ID or B's address, phone, website, coordinates or rating.
func assertChosenRestaurantPlaceOwnedElsewhereRejected(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	chosen := createPlaceTestRestaurant(t, store, placeTestInput(t, store, "Corner Noodles"))
	ownerInput := placeTestInput(t, store, "Harbor Grill")
	ownerInput.Address = "40 Wharf Street"
	ownerInput.City = "Wilmington"
	ownerInput.Category = "Seafood"
	ownerInput.GooglePlaceID = "place-harbor"
	ownerInput.GoogleMetadata = googleOwnedMetadata()
	owner := createPlaceTestRestaurant(t, store, ownerInput)
	visits := storeVisitCount(t, store)

	conflicting := placeTestInput(t, store, "Corner Noodles")
	conflicting.RestaurantID = &chosen.ID
	conflicting.Address = "40 Wharf Street"
	conflicting.City = "Wilmington"
	conflicting.Category = "Seafood"
	conflicting.GooglePlaceID = "place-harbor"
	conflicting.GoogleMetadata = googleOwnedMetadata()
	_, err := store.CreateVisit(ctx, conflicting)

	// Checked first: PR #83's reverted fix saved this visit but leaked B's metadata onto A.
	assertRestaurantUnchanged(t, "chosen restaurant", chosen, storeRestaurantByID(t, store, chosen.ID))
	assertRestaurantUnchanged(t, "owning restaurant", owner, storeRestaurantByID(t, store, owner.ID))
	var conflict *model.PlaceConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("CreateVisit error = %v, want PlaceConflictError", err)
	}
	if conflict.Restaurant != "Corner Noodles" || conflict.Owner != "Harbor Grill" {
		t.Fatalf("conflict = %#v, want Corner Noodles vs Harbor Grill", conflict)
	}
	if got := storeVisitCount(t, store); got != visits {
		t.Fatalf("visits = %d, want %d (rejected visit must not be saved)", got, visits)
	}
}

// assertChosenRestaurantDifferentPlaceRejected chooses restaurant A, already
// linked to one Google place, but submits another place's ID and metadata.
// No restaurant owns that place, but its details are not A's either.
func assertChosenRestaurantDifferentPlaceRejected(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	chosenInput := placeTestInput(t, store, "Linked Bistro")
	chosenInput.GooglePlaceID = "place-linked"
	chosen := createPlaceTestRestaurant(t, store, chosenInput)
	visits := storeVisitCount(t, store)

	other := placeTestInput(t, store, "Linked Bistro")
	other.RestaurantID = &chosen.ID
	other.Address = "77 Elsewhere Road"
	other.GooglePlaceID = "place-unlinked"
	other.GoogleMetadata = googleOwnedMetadata()
	_, err := store.CreateVisit(ctx, other)

	var conflict *model.PlaceConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("CreateVisit error = %v, want PlaceConflictError", err)
	}
	if conflict.Restaurant != "Linked Bistro" || conflict.Owner != "" {
		t.Fatalf("conflict = %#v, want Linked Bistro with no owner", conflict)
	}
	assertRestaurantUnchanged(t, "chosen restaurant", chosen, storeRestaurantByID(t, store, chosen.ID))
	if got := storeVisitCount(t, store); got != visits {
		t.Fatalf("visits = %d, want %d (rejected visit must not be saved)", got, visits)
	}
}

// assertChosenRestaurantMatchingPlaceAccepted keeps the allowed cases: the
// chosen restaurant's own place ID, no place ID, and an unowned place ID for a
// restaurant without one (which links it and fills its missing details).
func assertChosenRestaurantMatchingPlaceAccepted(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	linkedInput := placeTestInput(t, store, "Own Place Diner")
	linkedInput.GooglePlaceID = "place-own"
	linked := createPlaceTestRestaurant(t, store, linkedInput)

	same := placeTestInput(t, store, "Own Place Diner")
	same.RestaurantID = &linked.ID
	same.GooglePlaceID = "place-own"
	same.GoogleMetadata = googleOwnedMetadata()
	id, err := store.CreateVisit(ctx, same)
	if err != nil {
		t.Fatalf("own place ID: %v", err)
	}
	if got := storeRestaurant(t, store, *id); got.ID != linked.ID || got.Phone == nil || *got.Phone != "919-555-0199" {
		t.Fatalf("own place ID should fill missing details: %#v", got)
	}

	blank := placeTestInput(t, store, "Own Place Diner")
	blank.RestaurantID = &linked.ID
	if _, err := store.CreateVisit(ctx, blank); err != nil {
		t.Fatalf("no place ID: %v", err)
	}

	unlinked := createPlaceTestRestaurant(t, store, placeTestInput(t, store, "Unlinked Cafe"))
	link := placeTestInput(t, store, "Unlinked Cafe")
	link.RestaurantID = &unlinked.ID
	link.GooglePlaceID = "place-new"
	link.GoogleMetadata = googleOwnedMetadata()
	id, err = store.CreateVisit(ctx, link)
	if err != nil {
		t.Fatalf("unowned place ID: %v", err)
	}
	got := storeRestaurant(t, store, *id)
	if got.ID != unlinked.ID || got.GooglePlaceID == nil || *got.GooglePlaceID != "place-new" || got.Website == nil || *got.Website != "https://owner.example" {
		t.Fatalf("unowned place ID should link the chosen restaurant: %#v", got)
	}
}

// assertEditVisitIgnoresPlaceID edits a visit with another restaurant's place
// ID in the form. Editing never writes restaurant details, so it saves and
// neither restaurant changes.
func assertEditVisitIgnoresPlaceID(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	visitID, err := store.CreateVisit(ctx, placeTestInput(t, store, "Edited Tavern"))
	if err != nil {
		t.Fatal(err)
	}
	chosen := storeRestaurant(t, store, *visitID)
	ownerInput := placeTestInput(t, store, "Owner Tavern")
	ownerInput.GooglePlaceID = "place-owner-tavern"
	ownerInput.GoogleMetadata = googleOwnedMetadata()
	owner := createPlaceTestRestaurant(t, store, ownerInput)

	edit := placeTestInput(t, store, "Edited Tavern")
	edit.RestaurantID = &chosen.ID
	edit.GooglePlaceID = "place-owner-tavern"
	edit.GoogleMetadata = googleOwnedMetadata()
	edit.Notes = "edited"
	if err := store.UpdateVisit(ctx, *visitID, edit); err != nil {
		t.Fatalf("UpdateVisit: %v", err)
	}
	assertRestaurantUnchanged(t, "chosen restaurant", chosen, storeRestaurantByID(t, store, chosen.ID))
	assertRestaurantUnchanged(t, "owning restaurant", owner, storeRestaurantByID(t, store, owner.ID))
}
