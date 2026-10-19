package repository

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

// These checks run against both stores so the memory preview keeps the
// Postgres picker and aggregate rules. Each expects a store with the four
// seeded people and no visits.

func storePeople(t *testing.T, store DinerStore) (daniel, jen, caleb, aiden model.Person) {
	t.Helper()
	people, err := store.People(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(people) != 4 {
		t.Fatalf("got %d people, want 4", len(people))
	}
	return people[0], people[1], people[2], people[3]
}

func createPickTestVisit(t *testing.T, store DinerStore, name, category string, day int, picker *model.Person, ratings map[uuid.UUID]float64) uuid.UUID {
	t.Helper()
	input := model.VisitInput{
		RestaurantName: name,
		Address:        "1 " + name + " Way", // Same name and address: the same restaurant.
		Category:       category,
		VisitedAt:      time.Date(2026, 9, day, 18, 0, 0, 0, time.UTC),
		PriceLevel:     2,
		Ratings:        ratings,
	}
	if picker != nil {
		input.PickerID = &picker.ID
	}
	id, err := store.CreateVisit(context.Background(), input)
	if err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	return *id
}

func pickerNameOf(visit model.Visit) string {
	if visit.Picker == nil {
		return "Everybody"
	}
	return visit.Picker.Name
}

func assertBackToBackPicks(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	daniel, jen, _, _ := storePeople(t, store)
	createPickTestVisit(t, store, "First Pick", "", 1, &jen, map[uuid.UUID]float64{daniel.ID: 8})
	createPickTestVisit(t, store, "Second Pick", "", 2, &daniel, map[uuid.UUID]float64{daniel.ID: 8})
	createPickTestVisit(t, store, "Third Pick", "", 3, &daniel, map[uuid.UUID]float64{daniel.ID: 8})

	visits, err := store.Visits(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, visit := range visits {
		got = append(got, visit.Restaurant.Name+"/"+pickerNameOf(visit))
	}
	want := []string{"Third Pick/Daniel", "Second Pick/Daniel", "First Pick/Jen"}
	if len(got) != len(want) {
		t.Fatalf("visits = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("visits = %v, want %v", got, want)
		}
	}
}

func assertEverybodyPicks(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	daniel, jen, _, _ := storePeople(t, store)
	ratings := map[uuid.UUID]float64{daniel.ID: 8, jen.ID: 9}
	id := createPickTestVisit(t, store, "Group Outing", "", 1, nil, ratings)

	pickerOf := func() string {
		t.Helper()
		visit, err := store.Visit(ctx, id)
		if err != nil || visit == nil {
			t.Fatalf("read visit: %v, %v", visit, err)
		}
		return pickerNameOf(*visit)
	}
	if got := pickerOf(); got != "Everybody" {
		t.Fatalf("created picker = %s, want Everybody", got)
	}
	if visits, err := store.Visits(ctx, 0); err != nil || len(visits) != 1 || visits[0].Picker != nil {
		t.Fatalf("listed visits = %#v, err = %v; want the Everybody visit", visits, err)
	}

	visit, err := store.Visit(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	update := model.VisitInput{
		RestaurantID: &visit.Restaurant.ID,
		VisitedAt:    visit.VisitedAt,
		PickerID:     &jen.ID,
		PriceLevel:   2,
		Ratings:      ratings,
	}
	if err := store.UpdateVisit(ctx, id, update); err != nil {
		t.Fatal(err)
	}
	if got := pickerOf(); got != "Jen" {
		t.Fatalf("picker after edit = %s, want Jen", got)
	}
	update.PickerID = nil
	if err := store.UpdateVisit(ctx, id, update); err != nil {
		t.Fatal(err)
	}
	if got := pickerOf(); got != "Everybody" {
		t.Fatalf("picker after second edit = %s, want Everybody", got)
	}
}

func assertUnknownPickerRejected(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	daniel, _, _, _ := storePeople(t, store)
	unknown := uuid.New()
	_, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Stranger's Pick",
		VisitedAt:      time.Date(2026, 9, 1, 18, 0, 0, 0, time.UTC),
		PickerID:       &unknown,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{daniel.ID: 8},
	})
	if !errors.Is(err, model.ErrUnknownPicker) {
		t.Fatalf("create err = %v, want ErrUnknownPicker", err)
	}
	if restaurants, err := store.Restaurants(ctx, "Stranger's Pick"); err != nil || len(restaurants) != 0 {
		t.Fatalf("rejected create left restaurants %#v, err = %v", restaurants, err)
	}

	id := createPickTestVisit(t, store, "Known Pick", "", 2, &daniel, map[uuid.UUID]float64{daniel.ID: 8})
	visit, err := store.Visit(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	err = store.UpdateVisit(ctx, id, model.VisitInput{
		RestaurantID: &visit.Restaurant.ID,
		VisitedAt:    visit.VisitedAt,
		PickerID:     &unknown,
		PriceLevel:   2,
		Ratings:      map[uuid.UUID]float64{daniel.ID: 8},
	})
	if !errors.Is(err, model.ErrUnknownPicker) {
		t.Fatalf("update err = %v, want ErrUnknownPicker", err)
	}
	if visit, err := store.Visit(ctx, id); err != nil || pickerNameOf(*visit) != "Daniel" {
		t.Fatalf("rejected update changed the picker: %#v, err = %v", visit, err)
	}
}

// assertAggregatesNeedTwoRaters checks that only visits rated by at least two
// people count in averages and rankings, and that Everybody visits count in
// every aggregate except the per-person picker ones.
func assertAggregatesNeedTwoRaters(t *testing.T, store DinerStore) {
	t.Helper()
	daniel, jen, caleb, aiden := storePeople(t, store)
	// Two single-rater visits: with them, Solo Spot would top every list and
	// Daniel would be the best picker.
	createPickTestVisit(t, store, "Solo Spot", "Indian", 1, &daniel, map[uuid.UUID]float64{daniel.ID: 10})
	createPickTestVisit(t, store, "Solo Spot", "Indian", 2, &daniel, map[uuid.UUID]float64{jen.ID: 10})
	createPickTestVisit(t, store, "Pair Place", "Italian", 3, &daniel, map[uuid.UUID]float64{daniel.ID: 6, jen.ID: 7})
	// A single-rater visit to a ranked restaurant adds nothing to its ranking.
	createPickTestVisit(t, store, "Pair Place", "Italian", 7, nil, map[uuid.UUID]float64{caleb.ID: 1})
	createPickTestVisit(t, store, "Group Grill", "American", 4, nil, map[uuid.UUID]float64{daniel.ID: 9, jen.ID: 9, caleb.ID: 9})
	createPickTestVisit(t, store, "Jen's Pick", "Mexican", 5, &jen, map[uuid.UUID]float64{daniel.ID: 8, jen.ID: 8})
	createPickTestVisit(t, store, "Taco Two", "Mexican", 6, &jen, map[uuid.UUID]float64{caleb.ID: 7, aiden.ID: 7})

	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalDines != 7 || stats.NewPlaces != 5 {
		t.Fatalf("TotalDines/NewPlaces = %d/%d, want 7/5 (counts include every visit)", stats.TotalDines, stats.NewPlaces)
	}
	// Pair Place, Group Grill, Jen's Pick and Taco Two: 70 over 9 ratings.
	if math.Abs(stats.AverageRating-70.0/9.0) > 0.001 {
		t.Fatalf("AverageRating = %f, want %f", stats.AverageRating, 70.0/9.0)
	}
	// Daniel's only counted pick is Pair Place; Group Grill is no one's pick.
	if stats.BestPicker != "Jen" || math.Abs(stats.BestPickerAverage-7.5) > 0.001 {
		t.Fatalf("best picker = %s %f, want Jen 7.5", stats.BestPicker, stats.BestPickerAverage)
	}
	if stats.WorstPicker != "Daniel" || math.Abs(stats.WorstPickerAverage-6.5) > 0.001 {
		t.Fatalf("worst picker = %s %f, want Daniel 6.5", stats.WorstPicker, stats.WorstPickerAverage)
	}

	wantTop := []string{"Group Grill", "Jen's Pick", "Taco Two", "Pair Place"}
	if len(stats.TopRestaurants) != len(wantTop) {
		t.Fatalf("top restaurants = %#v, want %v", stats.TopRestaurants, wantTop)
	}
	for i, name := range wantTop {
		if stats.TopRestaurants[i].Name != name {
			t.Fatalf("top restaurants = %#v, want %v", stats.TopRestaurants, wantTop)
		}
	}
	if top := stats.TopRestaurants[0]; top.RatingCount != 3 || top.VisitCount != 1 {
		t.Fatalf("Group Grill counts = %d ratings/%d visits, want 3/1", top.RatingCount, top.VisitCount)
	}
	if pair := stats.TopRestaurants[3]; pair.RatingCount != 2 || pair.VisitCount != 1 || math.Abs(pair.AverageRating-6.5) > 0.001 {
		t.Fatalf("Pair Place = %#v, want 2 ratings over 1 counted visit averaging 6.5", pair)
	}

	wantCuisine := []string{"American/Group Grill", "Italian/Pair Place", "Mexican/Jen's Pick"}
	if len(stats.TopRestaurantsByCuisine) != len(wantCuisine) {
		t.Fatalf("cuisine winners = %#v, want %v", stats.TopRestaurantsByCuisine, wantCuisine)
	}
	for i, want := range wantCuisine {
		got := stats.TopRestaurantsByCuisine[i]
		if got.Cuisine+"/"+got.Name != want {
			t.Fatalf("cuisine winners = %#v, want %v", stats.TopRestaurantsByCuisine, wantCuisine)
		}
	}
	if italian := stats.TopRestaurantsByCuisine[1]; italian.RatingCount != 2 || italian.VisitCount != 1 {
		t.Fatalf("Italian winner = %#v, want 2 ratings over 1 counted visit", italian)
	}
}

func assertNoAggregatesWithoutTwoRaters(t *testing.T, store DinerStore) {
	t.Helper()
	daniel, _, _, _ := storePeople(t, store)
	createPickTestVisit(t, store, "Solo Spot", "Indian", 1, &daniel, map[uuid.UUID]float64{daniel.ID: 10})

	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalDines != 1 || stats.AverageRating != 0 || stats.BestPicker != "" || stats.WorstPicker != "" ||
		len(stats.TopRestaurants) != 0 || len(stats.TopRestaurantsByCuisine) != 0 {
		t.Fatalf("stats = %#v, want one dine and no rating aggregates", stats)
	}
	// One single-rater visit: no split, no repeat, nowhere dined since.
	if stats.BiggestSplitRestaurant != "" || stats.MostLoyalRestaurant != "" || stats.NostalgiaRestaurant != "" ||
		len(stats.TasteProfiles) != 0 {
		t.Fatalf("stats = %#v, want no trophy records", stats)
	}
}

// assertTrophyRecords checks the biggest rating split, most loyal, nostalgia
// and taste profile records.
func assertTrophyRecords(t *testing.T, store DinerStore) {
	t.Helper()
	daniel, jen, caleb, aiden := storePeople(t, store)
	createPickTestVisit(t, store, "Taco Town", "Mexican", 1, &daniel, map[uuid.UUID]float64{daniel.ID: 9, jen.ID: 3})
	createPickTestVisit(t, store, "Pho Place", "Vietnamese", 2, &jen, map[uuid.UUID]float64{daniel.ID: 6, jen.ID: 10})
	// Ties Taco Town's 6 point split on a newer visit, so it wins.
	createPickTestVisit(t, store, "Old Diner", "American", 3, &caleb, map[uuid.UUID]float64{daniel.ID: 10, jen.ID: 4, caleb.ID: 4})
	// A single rater counts toward no one's taste profile.
	createPickTestVisit(t, store, "Solo Sushi", "Japanese", 4, &caleb, map[uuid.UUID]float64{caleb.ID: 10})
	createPickTestVisit(t, store, "Taco Town", "Mexican", 5, &aiden, map[uuid.UUID]float64{daniel.ID: 8, jen.ID: 8, caleb.ID: 8})
	createPickTestVisit(t, store, "Pizza Pal", "Italian", 8, &aiden, map[uuid.UUID]float64{daniel.ID: 5, jen.ID: 5, caleb.ID: 1})
	createPickTestVisit(t, store, "Taco Town", "Mexican", 9, nil, map[uuid.UUID]float64{daniel.ID: 7, jen.ID: 9, caleb.ID: 9})
	// Caleb's best single score is Italian, but Mexican wins on average.
	createPickTestVisit(t, store, "Pizza Pal", "Italian", 10, &daniel, map[uuid.UUID]float64{daniel.ID: 5, caleb.ID: 10, aiden.ID: 5})

	stats, err := store.Stats(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.BiggestSplitRestaurant != "Old Diner" || math.Abs(stats.BiggestSplit-6) > 0.001 {
		t.Fatalf("biggest split = %s %f, want Old Diner 6", stats.BiggestSplitRestaurant, stats.BiggestSplit)
	}
	if stats.MostLoyalRestaurant != "Taco Town" || stats.MostLoyalVisits != 3 {
		t.Fatalf("most loyal = %s %d, want Taco Town 3", stats.MostLoyalRestaurant, stats.MostLoyalVisits)
	}
	wantSince := time.Date(2026, 9, 2, 18, 0, 0, 0, time.UTC)
	if stats.NostalgiaRestaurant != "Pho Place" || !stats.NostalgiaLastVisitedAt.Equal(wantSince) {
		t.Fatalf("nostalgia = %s %v, want Pho Place %v", stats.NostalgiaRestaurant, stats.NostalgiaLastVisitedAt, wantSince)
	}

	want := []model.PersonTasteStat{
		{Person: "Daniel", Category: "American", AverageRating: 10, RatingCount: 1},
		{Person: "Jen", Category: "Vietnamese", AverageRating: 10, RatingCount: 1},
		{Person: "Caleb", Category: "Mexican", AverageRating: 8.5, RatingCount: 2},
		{Person: "Aiden", Category: "Italian", AverageRating: 5, RatingCount: 1},
	}
	if len(stats.TasteProfiles) != len(want) {
		t.Fatalf("taste profiles = %#v, want %#v", stats.TasteProfiles, want)
	}
	for i, wantProfile := range want {
		got := stats.TasteProfiles[i]
		if got.Person != wantProfile.Person || got.Category != wantProfile.Category ||
			math.Abs(got.AverageRating-wantProfile.AverageRating) > 0.001 || got.RatingCount != wantProfile.RatingCount {
			t.Fatalf("taste profiles = %#v, want %#v", stats.TasteProfiles, want)
		}
	}
}
