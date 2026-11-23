package repository

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

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
	if stats.WorstPicker != "Daniel" {
		t.Fatalf("WorstPicker = %q, want Daniel", stats.WorstPicker)
	}
	if math.Abs(stats.WorstPickerAverage-7.75) > 0.001 {
		t.Fatalf("WorstPickerAverage = %f, want 7.75", stats.WorstPickerAverage)
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
	if stats.BiggestSplitRestaurant != "El Patio Verde" || math.Abs(stats.BiggestSplit-2) > 0.001 {
		t.Fatalf("biggest split = %s %f, want El Patio Verde 2", stats.BiggestSplitRestaurant, stats.BiggestSplit)
	}
	// Every demo restaurant has one visit, so none is a repeat.
	if stats.MostLoyalRestaurant != "" {
		t.Fatalf("MostLoyalRestaurant = %q, want none", stats.MostLoyalRestaurant)
	}
	if stats.NostalgiaRestaurant != "Saffron Counter" {
		t.Fatalf("NostalgiaRestaurant = %q, want Saffron Counter", stats.NostalgiaRestaurant)
	}
	if len(stats.TasteProfiles) != 4 || stats.TasteProfiles[0].Person != "Daniel" || stats.TasteProfiles[0].Category != "Mexican" {
		t.Fatalf("taste profiles = %#v, want four with Daniel's Mexican first", stats.TasteProfiles)
	}
}

func TestMemoryStoreTopRestaurantsRequireTwoRatingsAndSortDeterministically(t *testing.T) {
	// Solo's only visit has one rater, so it counts in no ranking.
	store := NewMemoryStore()
	people := store.people
	now := time.Now()
	abacus := model.Restaurant{ID: uuid.New(), Name: "Abacus", City: strPtr("Apex"), Category: strPtr("Italian")}
	alpha := model.Restaurant{ID: uuid.New(), Name: "Alpha", City: strPtr("Apex"), Category: strPtr("Italian")}
	bravo := model.Restaurant{ID: uuid.New(), Name: "Bravo", City: strPtr("Cary"), Category: strPtr("Mexican")}
	solo := model.Restaurant{ID: uuid.New(), Name: "Solo", City: strPtr("Raleigh"), Category: strPtr("Indian")}
	store.restaurants = []model.Restaurant{abacus, alpha, bravo, solo}
	store.visits = []model.Visit{
		demoVisit(alpha, &people[0], now, 2, "", []model.Rating{{Person: people[0], Score: 9}, {Person: people[1], Score: 8}}, nil),
		demoVisit(bravo, &people[1], now, 2, "", []model.Rating{{Person: people[0], Score: 8.5}, {Person: people[1], Score: 8.5}, {Person: people[2], Score: 8.5}}, nil),
		demoVisit(abacus, &people[2], now, 2, "", []model.Rating{{Person: people[0], Score: 8.5}, {Person: people[1], Score: 8.5}}, nil),
		demoVisit(solo, &people[3], now, 2, "", []model.Rating{{Person: people[0], Score: 10}}, nil),
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
	wantCuisine := []model.CuisineRestaurantStat{
		{Cuisine: "Italian", Name: "Abacus"},
		{Cuisine: "Mexican", Name: "Bravo"},
	}
	if len(stats.TopRestaurantsByCuisine) != len(wantCuisine) {
		t.Fatalf("got %d cuisine winners, want %d: %#v", len(stats.TopRestaurantsByCuisine), len(wantCuisine), stats.TopRestaurantsByCuisine)
	}
	for i, want := range wantCuisine {
		got := stats.TopRestaurantsByCuisine[i]
		if got.Cuisine != want.Cuisine || got.Name != want.Name {
			t.Fatalf("cuisine winner %d = %s/%s, want %s/%s", i, got.Cuisine, got.Name, want.Cuisine, want.Name)
		}
	}
}
