package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

// These checks run against both stores so the memory preview keeps the
// Postgres Want to Try rules. Each expects a store with the four seeded
// people and no restaurants or visits.

func TestMemoryStoreWantToTrySaveListAndDelete(t *testing.T) {
	assertWantToTrySaveListAndDelete(t, emptyMemoryStore())
}

func TestPostgresWantToTrySaveListAndDelete(t *testing.T) {
	assertWantToTrySaveListAndDelete(t, postgresStore(t))
}

func TestMemoryStoreWantToTryClearsWhenDineLogged(t *testing.T) {
	assertWantToTryClearsWhenDineLogged(t, emptyMemoryStore())
}

func TestPostgresWantToTryClearsWhenDineLogged(t *testing.T) {
	assertWantToTryClearsWhenDineLogged(t, postgresStore(t))
}

func TestMemoryStoreWantToTryClearsWhenDineMovedThere(t *testing.T) {
	assertWantToTryClearsWhenDineMovedThere(t, emptyMemoryStore())
}

func TestPostgresWantToTryClearsWhenDineMovedThere(t *testing.T) {
	assertWantToTryClearsWhenDineMovedThere(t, postgresStore(t))
}

func TestMemoryStoreWantToTryRejectsBadInput(t *testing.T) {
	assertWantToTryRejectsBadInput(t, emptyMemoryStore())
}

func TestPostgresWantToTryRejectsBadInput(t *testing.T) {
	assertWantToTryRejectsBadInput(t, postgresStore(t))
}

func TestMemoryStoreWantToTryGoesWithDeletedRestaurant(t *testing.T) {
	assertWantToTryGoesWithDeletedRestaurant(t, emptyMemoryStore())
}

func TestPostgresWantToTryGoesWithDeletedRestaurant(t *testing.T) {
	assertWantToTryGoesWithDeletedRestaurant(t, postgresStore(t))
}

func saveWantToTry(t *testing.T, store DinerStore, input model.WantToTryInput) uuid.UUID {
	t.Helper()
	id, err := store.SaveWantToTry(context.Background(), input)
	if err != nil {
		t.Fatalf("save want to try %+v: %v", input, err)
	}
	return *id
}

func wantToTryNames(t *testing.T, store DinerStore) []string {
	t.Helper()
	items, err := store.WantToTry(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, item := range items {
		names = append(names, item.Restaurant.Name)
	}
	return names
}

func assertWantToTrySaveListAndDelete(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	_, jen, caleb, _ := storePeople(t, store)
	rating := 4.6
	priceLevel := 2

	// A Google place nobody has saved becomes a new restaurant with its details.
	noodleID := saveWantToTry(t, store, model.WantToTryInput{
		RestaurantName: "Lucky Noodle Bar",
		Address:        "1 Noodle Way",
		City:           "Dublin",
		GooglePlaceID:  "place-noodle",
		GoogleMetadata: model.GoogleRestaurantMetadata{GoogleRating: &rating, GooglePriceLevel: &priceLevel},
		Category:       "Japanese",
		SuggestedByID:  &caleb.ID,
		Note:           "  Spicy miso ramen.  ",
	})
	time.Sleep(2 * time.Millisecond)
	visitID := createPickTestVisit(t, store, "Bluebird Biscuit", "Breakfast", 1, &jen, map[uuid.UUID]float64{jen.ID: 8})
	visit, err := store.Visit(ctx, visitID)
	if err != nil {
		t.Fatal(err)
	}
	// A saved restaurant chosen by ID, as on its page; Everybody's idea, no note.
	biscuitID := saveWantToTry(t, store, model.WantToTryInput{RestaurantID: &visit.Restaurant.ID})

	if got := wantToTryNames(t, store); strings.Join(got, ",") != "Bluebird Biscuit,Lucky Noodle Bar" {
		t.Fatalf("list = %v, want newest first", got)
	}
	noodle, err := store.WantToTryItem(ctx, noodleID)
	if err != nil || noodle == nil {
		t.Fatalf("item = %#v, err = %v", noodle, err)
	}
	if noodle.SuggestedBy == nil || noodle.SuggestedBy.Name != "Caleb" || noodle.Note == nil || *noodle.Note != "Spicy miso ramen." || noodle.DoneAt != nil {
		t.Fatalf("noodle item = %#v, want Caleb's open idea with a trimmed note", noodle)
	}
	if r := noodle.Restaurant; r.GooglePlaceID == nil || *r.GooglePlaceID != "place-noodle" || r.City == nil || *r.City != "Dublin" ||
		r.Category == nil || *r.Category != "Japanese" || r.GoogleRating == nil || *r.GoogleRating != rating {
		t.Fatalf("noodle restaurant = %#v, want the place's details", r)
	}
	biscuit, err := store.WantToTryItem(ctx, biscuitID)
	if err != nil || biscuit == nil || biscuit.SuggestedBy != nil || biscuit.Note != nil || biscuit.Restaurant.ID != visit.Restaurant.ID {
		t.Fatalf("biscuit item = %#v, err = %v; want Everybody's idea on the chosen restaurant", biscuit, err)
	}

	// Saving the same place again updates the item rather than adding one.
	again := saveWantToTry(t, store, model.WantToTryInput{
		RestaurantName: "Lucky Noodle Bar",
		GooglePlaceID:  "place-noodle",
		SuggestedByID:  &jen.ID,
		Note:           "Jen heard the gyoza is great.",
	})
	if again != noodleID {
		t.Fatalf("re-save id = %s, want the existing %s", again, noodleID)
	}
	noodle, err = store.WantToTryItem(ctx, noodleID)
	if err != nil || noodle.SuggestedBy == nil || noodle.SuggestedBy.Name != "Jen" || *noodle.Note != "Jen heard the gyoza is great." {
		t.Fatalf("re-saved item = %#v, err = %v; want Jen's note", noodle, err)
	}
	if got := wantToTryNames(t, store); strings.Join(got, ",") != "Bluebird Biscuit,Lucky Noodle Bar" {
		t.Fatalf("list after re-save = %v, want the same two items in the same order", got)
	}
	if restaurants, err := store.Restaurants(ctx, "Lucky Noodle"); err != nil || len(restaurants) != 1 {
		t.Fatalf("restaurants = %#v, err = %v; want one Lucky Noodle Bar", restaurants, err)
	}

	if err := store.DeleteWantToTry(ctx, noodleID); err != nil {
		t.Fatal(err)
	}
	if got := wantToTryNames(t, store); strings.Join(got, ",") != "Bluebird Biscuit" {
		t.Fatalf("list after delete = %v", got)
	}
	if item, err := store.WantToTryItem(ctx, noodleID); err != nil || item != nil {
		t.Fatalf("deleted item = %#v, err = %v", item, err)
	}
	if restaurants, err := store.Restaurants(ctx, "Lucky Noodle"); err != nil || len(restaurants) != 1 {
		t.Fatalf("removing the item should keep the restaurant: %#v, err = %v", restaurants, err)
	}
}

func assertWantToTryClearsWhenDineLogged(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	daniel, _, caleb, _ := storePeople(t, store)
	id := saveWantToTry(t, store, model.WantToTryInput{
		RestaurantName: "Smoke & Pine",
		Address:        "7 Pine St",
		GooglePlaceID:  "place-smoke",
		SuggestedByID:  &daniel.ID,
	})
	keepID := saveWantToTry(t, store, model.WantToTryInput{RestaurantName: "Rolling Pin", Address: "9 Pie Ln"})

	// Any dine logged there clears it, even one found by place ID rather than chosen.
	_, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Smoke and Pine BBQ",
		GooglePlaceID:  "place-smoke",
		VisitedAt:      time.Date(2026, 9, 5, 18, 0, 0, 0, time.UTC),
		PickerID:       &daniel.ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{daniel.ID: 9},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := wantToTryNames(t, store); strings.Join(got, ",") != "Rolling Pin" {
		t.Fatalf("list after dine = %v, want only Rolling Pin", got)
	}
	done, err := store.WantToTryItem(ctx, id)
	if err != nil || done == nil || done.DoneAt == nil {
		t.Fatalf("logged item = %#v, err = %v; want it done", done, err)
	}
	if restaurants, err := store.Restaurants(ctx, "Smoke"); err != nil || len(restaurants) != 1 {
		t.Fatalf("restaurants = %#v, err = %v; want the dine to reuse the saved restaurant", restaurants, err)
	}

	// Saving it again puts it back on the list as a new idea.
	time.Sleep(2 * time.Millisecond)
	again := saveWantToTry(t, store, model.WantToTryInput{RestaurantID: &done.Restaurant.ID, SuggestedByID: &caleb.ID, Note: "Back for ribs"})
	if again != id {
		t.Fatalf("re-save id = %s, want %s", again, id)
	}
	reopened, err := store.WantToTryItem(ctx, id)
	if err != nil || reopened.DoneAt != nil || !reopened.CreatedAt.After(done.CreatedAt) || reopened.SuggestedBy == nil || reopened.SuggestedBy.Name != "Caleb" {
		t.Fatalf("reopened item = %#v, err = %v; want an open, newer item that is Caleb's idea", reopened, err)
	}
	if got := wantToTryNames(t, store); strings.Join(got, ",") != "Smoke & Pine,Rolling Pin" {
		t.Fatalf("list after reopening = %v, want Smoke & Pine first", got)
	}
	if item, err := store.WantToTryItem(ctx, keepID); err != nil || item.DoneAt != nil {
		t.Fatalf("unrelated item = %#v, err = %v; want it still open", item, err)
	}
}

func assertWantToTryClearsWhenDineMovedThere(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	daniel, _, _, _ := storePeople(t, store)
	ratings := map[uuid.UUID]float64{daniel.ID: 8}
	visitID := createPickTestVisit(t, store, "Old Haunt", "", 1, &daniel, ratings)
	visit, err := store.Visit(ctx, visitID)
	if err != nil {
		t.Fatal(err)
	}
	// The dine's own restaurant goes back on the list ("want to go back").
	backID := saveWantToTry(t, store, model.WantToTryInput{RestaurantID: &visit.Restaurant.ID})
	newID := saveWantToTry(t, store, model.WantToTryInput{RestaurantName: "New Spot", Address: "2 New St"})
	newItem, err := store.WantToTryItem(ctx, newID)
	if err != nil {
		t.Fatal(err)
	}

	edit := model.VisitInput{RestaurantID: &visit.Restaurant.ID, VisitedAt: visit.VisitedAt, PickerID: &daniel.ID, PriceLevel: 2, Ratings: ratings, Notes: "Edited"}
	if err := store.UpdateVisit(ctx, visitID, edit); err != nil {
		t.Fatal(err)
	}
	if got := wantToTryNames(t, store); strings.Join(got, ",") != "New Spot,Old Haunt" {
		t.Fatalf("editing a dine in place changed the list to %v", got)
	}

	edit.RestaurantID = &newItem.Restaurant.ID
	if err := store.UpdateVisit(ctx, visitID, edit); err != nil {
		t.Fatal(err)
	}
	if got := wantToTryNames(t, store); strings.Join(got, ",") != "Old Haunt" {
		t.Fatalf("list after moving the dine = %v, want New Spot cleared", got)
	}
	if item, err := store.WantToTryItem(ctx, newID); err != nil || item.DoneAt == nil {
		t.Fatalf("moved-to item = %#v, err = %v; want it done", item, err)
	}
	if item, err := store.WantToTryItem(ctx, backID); err != nil || item.DoneAt != nil {
		t.Fatalf("moved-from item = %#v, err = %v; want it still open", item, err)
	}
}

func assertWantToTryRejectsBadInput(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	unknown := uuid.New()
	if _, err := store.SaveWantToTry(ctx, model.WantToTryInput{RestaurantName: "Stranger Spot", SuggestedByID: &unknown}); !errors.Is(err, model.ErrUnknownPicker) {
		t.Fatalf("unknown suggester err = %v, want ErrUnknownPicker", err)
	}
	if restaurants, err := store.Restaurants(ctx, "Stranger Spot"); err != nil || len(restaurants) != 0 {
		t.Fatalf("rejected save left restaurants %#v, err = %v", restaurants, err)
	}
	if _, err := store.SaveWantToTry(ctx, model.WantToTryInput{RestaurantID: &unknown}); !errors.Is(err, model.ErrRestaurantNotFound) {
		t.Fatalf("missing restaurant err = %v, want ErrRestaurantNotFound", err)
	}
	if _, err := store.SaveWantToTry(ctx, model.WantToTryInput{RestaurantName: "  "}); err == nil {
		t.Fatal("save without a restaurant succeeded")
	}
	long := strings.Repeat("é", model.MaxWantToTryNoteLength+1)
	if _, err := store.SaveWantToTry(ctx, model.WantToTryInput{RestaurantName: "Long Note", Note: long}); err == nil {
		t.Fatal("save with an overlong note succeeded")
	}
	if got := wantToTryNames(t, store); len(got) != 0 {
		t.Fatalf("rejected saves listed %v", got)
	}
}

func assertWantToTryGoesWithDeletedRestaurant(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	id := saveWantToTry(t, store, model.WantToTryInput{RestaurantName: "Third Street Deli", Address: "62 N 3rd St"})
	item, err := store.WantToTryItem(ctx, id)
	if err != nil || item == nil {
		t.Fatalf("item = %#v, err = %v", item, err)
	}
	deleted, err := store.DeleteRestaurantIfUnvisited(ctx, item.Restaurant.ID)
	if err != nil || !deleted {
		t.Fatalf("delete restaurant = %v, err = %v", deleted, err)
	}
	if got := wantToTryNames(t, store); len(got) != 0 {
		t.Fatalf("list after deleting its restaurant = %v", got)
	}
	if item, err := store.WantToTryItem(ctx, id); err != nil || item != nil {
		t.Fatalf("item after deleting its restaurant = %#v, err = %v", item, err)
	}
}
