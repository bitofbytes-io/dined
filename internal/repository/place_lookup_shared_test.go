package repository

import (
	"context"
	"testing"
)

// assertRestaurantsByPlaceIDs checks that a batch of Google place IDs finds
// the saved restaurants linked to them and leaves out unknown IDs. It runs
// against both stores and expects the seeded people and no visits.
func assertRestaurantsByPlaceIDs(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	first := placeTestInput(t, store, "Linked Diner")
	first.GooglePlaceID = "place-linked"
	linked := createPlaceTestRestaurant(t, store, first)
	if _, err := store.CreateVisit(ctx, first); err != nil {
		t.Fatalf("second visit: %v", err)
	}
	other := placeTestInput(t, store, "Other Grill")
	other.GooglePlaceID = "place-other"
	otherRestaurant := createPlaceTestRestaurant(t, store, other)
	createPlaceTestRestaurant(t, store, placeTestInput(t, store, "No Place Cafe"))

	found, err := store.RestaurantsByPlaceIDs(ctx, []string{"place-linked", "place-other", "place-unknown"})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 {
		t.Fatalf("found %d restaurants, want 2: %#v", len(found), found)
	}
	if got := found["place-linked"]; got.ID != linked.ID || got.Name != "Linked Diner" {
		t.Fatalf("place-linked = %#v, want %s", got, linked.ID)
	}
	if got := found["place-other"]; got.ID != otherRestaurant.ID {
		t.Fatalf("place-other = %#v, want %s", got, otherRestaurant.ID)
	}

	found, err = store.RestaurantsByPlaceIDs(ctx, nil)
	if err != nil || len(found) != 0 {
		t.Fatalf("no IDs found %#v, err = %v; want none", found, err)
	}
}
