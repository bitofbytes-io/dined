package repository

import (
	"context"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

// These checks run against both stores so the memory preview keeps the
// Postgres rules for permanently closed restaurants. Each expects a store
// with the four seeded people and no restaurants or visits.

func TestMemoryStoreClosedRestaurantsLeaveSpinnerAndWantToTry(t *testing.T) {
	assertClosedRestaurantsLeaveSpinnerAndWantToTry(t, emptyMemoryStore())
}

func TestPostgresClosedRestaurantsLeaveSpinnerAndWantToTry(t *testing.T) {
	assertClosedRestaurantsLeaveSpinnerAndWantToTry(t, postgresStore(t))
}

func TestMemoryStoreClosedRestaurantsKeepHistory(t *testing.T) {
	assertClosedRestaurantsKeepHistory(t, emptyMemoryStore())
}

func TestPostgresClosedRestaurantsKeepHistory(t *testing.T) {
	assertClosedRestaurantsKeepHistory(t, postgresStore(t))
}

// setRestaurantClosed saves the restaurant unchanged except for its closed flag.
func setRestaurantClosed(t *testing.T, store DinerStore, id uuid.UUID, closed bool) model.Restaurant {
	t.Helper()
	ctx := context.Background()
	restaurant, err := store.Restaurant(ctx, id)
	if err != nil || restaurant == nil {
		t.Fatalf("restaurant = %#v, err = %v", restaurant, err)
	}
	input := model.RestaurantInput{
		Name:             restaurant.Name,
		Address:          valueOrEmpty(restaurant.Address),
		City:             valueOrEmpty(restaurant.City),
		Phone:            valueOrEmpty(restaurant.Phone),
		Website:          valueOrEmpty(restaurant.Website),
		GoogleRating:     restaurant.GoogleRating,
		GooglePriceLevel: restaurant.GooglePriceLevel,
		Category:         valueOrEmpty(restaurant.Category),
		IsChain:          restaurant.IsChain,
		Closed:           closed,
	}
	if err := store.UpdateRestaurant(ctx, id, input); err != nil {
		t.Fatalf("update restaurant closed=%v: %v", closed, err)
	}
	updated, err := store.Restaurant(ctx, id)
	if err != nil || updated == nil {
		t.Fatalf("restaurant after update = %#v, err = %v", updated, err)
	}
	return *updated
}

func restedFavoriteNames(t *testing.T, store DinerStore) []string {
	t.Helper()
	favorites, err := store.RestedFavorites(context.Background(), 7, spinTestNow.AddDate(0, 0, -30))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, favorite := range favorites {
		names = append(names, favorite.Restaurant.Name)
	}
	return names
}

func assertSameNames(t *testing.T, what string, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", what, got, want)
		}
	}
}

func assertClosedRestaurantsLeaveSpinnerAndWantToTry(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	daniel, jen, _, _ := storePeople(t, store)
	ratings := map[uuid.UUID]float64{daniel.ID: 9, jen.ID: 9}
	createSpinTestVisit(t, store, "Closed Favorite", 60, &daniel, ratings)
	createSpinTestVisit(t, store, "Open Favorite", 60, &jen, ratings)
	closedWant := saveWantToTry(t, store, model.WantToTryInput{RestaurantName: "Closed Want", Address: "1 Closed Way"})
	saveWantToTry(t, store, model.WantToTryInput{RestaurantName: "Open Want", Address: "2 Open Way"})

	assertSameNames(t, "favorites before closing", restedFavoriteNames(t, store), "Closed Favorite", "Open Favorite")
	assertSameNames(t, "want to try before closing", wantToTryNames(t, store), "Open Want", "Closed Want")

	for _, name := range []string{"Closed Favorite", "Closed Want"} {
		found, err := store.Restaurants(ctx, name)
		if err != nil || len(found) != 1 {
			t.Fatalf("restaurants %q = %#v, err = %v", name, found, err)
		}
		closed := setRestaurantClosed(t, store, found[0].ID, true)
		if !closed.IsClosed() || !closed.ClosedAt.Equal(closingDate(time.Now())) {
			t.Fatalf("%s closed at %v, want today's date", name, closed.ClosedAt)
		}
	}

	assertSameNames(t, "favorites after closing", restedFavoriteNames(t, store), "Open Favorite")
	assertSameNames(t, "want to try after closing", wantToTryNames(t, store), "Open Want")
	// The item is still there to open by ID, only the list leaves it out.
	if item, err := store.WantToTryItem(ctx, closedWant); err != nil || item == nil || !item.Restaurant.IsClosed() {
		t.Fatalf("closed want to try item = %#v, err = %v", item, err)
	}

	// Saving a closed restaurant does not bring it back on the list.
	closedID := mustRestaurantID(t, store, "Closed Want")
	saveWantToTry(t, store, model.WantToTryInput{RestaurantID: &closedID})
	assertSameNames(t, "want to try after saving a closed restaurant", wantToTryNames(t, store), "Open Want")

	// Reopening puts both back.
	for _, name := range []string{"Closed Favorite", "Closed Want"} {
		reopened := setRestaurantClosed(t, store, mustRestaurantID(t, store, name), false)
		if reopened.IsClosed() {
			t.Fatalf("%s still closed at %v after reopening", name, reopened.ClosedAt)
		}
	}
	assertSameNames(t, "favorites after reopening", restedFavoriteNames(t, store), "Closed Favorite", "Open Favorite")
	assertSameNames(t, "want to try after reopening", wantToTryNames(t, store), "Open Want", "Closed Want")
}

func mustRestaurantID(t *testing.T, store DinerStore, name string) uuid.UUID {
	t.Helper()
	found, err := store.Restaurants(context.Background(), name)
	if err != nil || len(found) != 1 {
		t.Fatalf("restaurants %q = %#v, err = %v", name, found, err)
	}
	return found[0].ID
}

func assertClosedRestaurantsKeepHistory(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	daniel, jen, _, _ := storePeople(t, store)
	ratings := map[uuid.UUID]float64{daniel.ID: 9, jen.ID: 8}
	createSpinTestVisit(t, store, "Closed Diner", 60, &daniel, ratings)
	createSpinTestVisit(t, store, "Closed Diner", 90, &jen, ratings)
	createSpinTestVisit(t, store, "Open Diner", 60, &jen, ratings)
	closedID := mustRestaurantID(t, store, "Closed Diner")

	before, err := store.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	closed := setRestaurantClosed(t, store, closedID, true)

	// Closing again keeps the original date.
	again := setRestaurantClosed(t, store, closedID, true)
	if !again.ClosedAt.Equal(*closed.ClosedAt) {
		t.Fatalf("closed again at %v, want original %v", again.ClosedAt, closed.ClosedAt)
	}

	visits, err := store.Visits(ctx, 10)
	if err != nil || len(visits) != 3 {
		t.Fatalf("visits = %d, err = %v, want all 3 kept", len(visits), err)
	}
	closedVisits := 0
	for _, visit := range visits {
		if visit.Restaurant.ID == closedID {
			closedVisits++
			if !visit.Restaurant.IsClosed() {
				t.Fatalf("visit at the closed restaurant has no closed date: %#v", visit.Restaurant)
			}
		}
	}
	if closedVisits != 2 {
		t.Fatalf("visits at the closed restaurant = %d, want 2", closedVisits)
	}
	if count, err := store.CountVisits(ctx, model.VisitFilter{}); err != nil || count != 3 {
		t.Fatalf("count visits = %d, err = %v, want 3", count, err)
	}
	history, err := store.RestaurantVisits(ctx, closedID)
	if err != nil || len(history) != 2 {
		t.Fatalf("restaurant visits = %d, err = %v, want 2", len(history), err)
	}
	if found, err := store.Restaurants(ctx, "Closed Diner"); err != nil || len(found) != 1 || !found[0].IsClosed() {
		t.Fatalf("search = %#v, err = %v, want the closed restaurant with its date", found, err)
	}

	after, err := store.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.TotalDines != before.TotalDines || after.NewPlaces != before.NewPlaces || len(after.TopRestaurants) != len(before.TopRestaurants) {
		t.Fatalf("stats after closing = %#v, want trophies unchanged from %#v", after, before)
	}
	if after.TotalDines != 3 {
		t.Fatalf("total dines = %d, want 3", after.TotalDines)
	}
	foundClosed := false
	for _, top := range after.TopRestaurants {
		foundClosed = foundClosed || top.Name == "Closed Diner"
	}
	if !foundClosed {
		t.Fatalf("top restaurants = %#v, want Closed Diner still counted", after.TopRestaurants)
	}
}
