package repository

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

// These checks run against both stores so the memory preview keeps the
// Postgres year in review rules. Each expects a store with the four seeded
// people and no visits.

func createYearTestVisit(t *testing.T, store DinerStore, name string, visitedAt time.Time, picker *model.Person, ratings map[uuid.UUID]float64) {
	t.Helper()
	input := model.VisitInput{
		RestaurantName: name,
		Address:        "1 " + name + " Way",
		VisitedAt:      visitedAt,
		PriceLevel:     2,
		Ratings:        ratings,
	}
	if picker != nil {
		input.PickerID = &picker.ID
	}
	if _, err := store.CreateVisit(context.Background(), input); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
}

func assertYearInReview(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	daniel, jen, caleb, aiden := storePeople(t, store)
	utc := func(year int, month time.Month, day, hour int) time.Time {
		return time.Date(year, month, day, hour, 0, 0, 0, time.UTC)
	}
	both := func(a, b float64) map[uuid.UUID]float64 { return map[uuid.UUID]float64{daniel.ID: a, jen.ID: b} }

	createYearTestVisit(t, store, "Old Haunt", utc(2024, time.June, 10, 18), &jen, both(8, 8))
	createYearTestVisit(t, store, "Old Haunt", utc(2025, time.January, 15, 18), &daniel, both(6, 6))
	createYearTestVisit(t, store, "Taco Town", utc(2025, time.March, 5, 18), &jen, both(9, 9))
	createYearTestVisit(t, store, "Taco Town", utc(2025, time.March, 20, 18), &jen, both(7, 9))
	// Everybody picks: they count in the year's totals but in no one's top pick.
	createYearTestVisit(t, store, "Pho Place", utc(2025, time.March, 25, 18), nil, both(10, 10))
	// One rater is not enough for the ratings to count.
	createYearTestVisit(t, store, "Solo Sushi", utc(2025, time.September, 8, 18), &caleb, map[uuid.UUID]float64{caleb.ID: 10})
	// 10pm on New Year's Eve in Eastern time, already January in UTC.
	createYearTestVisit(t, store, "Pizza Pal", utc(2026, time.January, 1, 3), &daniel, both(5, 5))
	createYearTestVisit(t, store, "Taco Town", utc(2026, time.February, 12, 18), &aiden, both(8, 8))

	review, err := store.YearInReview(ctx, 2025)
	if err != nil {
		t.Fatal(err)
	}
	if review.Year != 2025 || review.TotalVisits != 6 {
		t.Fatalf("2025 = year %d with %d visits, want 6", review.Year, review.TotalVisits)
	}
	wantMonths := [12]int{1, 0, 3, 0, 0, 0, 0, 0, 1, 0, 0, 1}
	if review.MonthlyVisits != wantMonths {
		t.Fatalf("2025 monthly visits = %v, want %v", review.MonthlyVisits, wantMonths)
	}
	if review.NewRestaurants != 4 || review.RepeatRestaurants != 1 {
		t.Fatalf("2025 new/repeat = %d/%d, want 4/1", review.NewRestaurants, review.RepeatRestaurants)
	}
	if review.MostVisited == nil || review.MostVisited.Name != "Taco Town" || review.MostVisited.VisitCount != 2 {
		t.Fatalf("2025 most visited = %#v, want Taco Town 2", review.MostVisited)
	}
	if review.HighestRated == nil || review.HighestRated.Name != "Pho Place" || math.Abs(review.HighestRated.AverageRating-10) > 0.001 {
		t.Fatalf("2025 highest rated = %#v, want Pho Place 10", review.HighestRated)
	}
	want := []model.PersonTopPick{
		{Person: "Daniel", Restaurant: "Old Haunt", AverageRating: 6},
		{Person: "Jen", Restaurant: "Taco Town", AverageRating: 8.5},
	}
	if len(review.TopPicks) != len(want) {
		t.Fatalf("2025 top picks = %#v, want %#v", review.TopPicks, want)
	}
	for i, wantPick := range want {
		got := review.TopPicks[i]
		if got.Person != wantPick.Person || got.Restaurant != wantPick.Restaurant || got.RestaurantID == uuid.Nil ||
			math.Abs(got.AverageRating-wantPick.AverageRating) > 0.001 {
			t.Fatalf("2025 top picks = %#v, want %#v", review.TopPicks, want)
		}
	}

	// Taco Town was new in 2025, so it is a repeat in 2026. One visit per
	// restaurant means there is no most visited.
	review, err = store.YearInReview(ctx, 2026)
	if err != nil {
		t.Fatal(err)
	}
	if review.TotalVisits != 1 || review.MonthlyVisits[1] != 1 || review.NewRestaurants != 0 || review.RepeatRestaurants != 1 {
		t.Fatalf("2026 = %#v, want one February visit to a repeat restaurant", review)
	}
	if review.MostVisited != nil {
		t.Fatalf("2026 most visited = %#v, want none", review.MostVisited)
	}
	if review.HighestRated == nil || review.HighestRated.Name != "Taco Town" || len(review.TopPicks) != 1 || review.TopPicks[0].Person != "Aiden" {
		t.Fatalf("2026 = %#v, want Taco Town highest rated and Aiden's top pick", review)
	}
}

// assertYearInReviewTiesBreakByRestaurantID checks that two same-name
// locations with identical visits and ratings always rank the lower ID first,
// so the restaurant a highlight links to never changes between loads.
func assertYearInReviewTiesBreakByRestaurantID(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	daniel, jen, _, _ := storePeople(t, store)
	for _, address := range []string{"1 North St", "2 South St"} {
		for _, day := range []int{5, 6} {
			input := model.VisitInput{
				RestaurantName: "Twin Grill",
				Address:        address,
				VisitedAt:      time.Date(2025, time.May, day, 18, 0, 0, 0, time.UTC),
				PriceLevel:     2,
				PickerID:       &daniel.ID,
				Ratings:        map[uuid.UUID]float64{daniel.ID: 8, jen.ID: 8},
			}
			if _, err := store.CreateVisit(ctx, input); err != nil {
				t.Fatal(err)
			}
		}
	}
	twins, err := store.Restaurants(ctx, "Twin Grill")
	if err != nil || len(twins) != 2 {
		t.Fatalf("twin restaurants = %#v, err = %v; want 2", twins, err)
	}
	want := twins[0].ID
	if twins[1].ID.String() < want.String() {
		want = twins[1].ID
	}

	review, err := store.YearInReview(ctx, 2025)
	if err != nil {
		t.Fatal(err)
	}
	if review.MostVisited == nil || review.MostVisited.ID != want ||
		review.HighestRated == nil || review.HighestRated.ID != want ||
		len(review.TopPicks) != 1 || review.TopPicks[0].RestaurantID != want {
		t.Fatalf("year in review = %#v, want every highlight on restaurant %s", review, want)
	}
}

// assertYearInReviewWithoutVisits checks a year with no visits, in a store
// that has visits in other years.
func assertYearInReviewWithoutVisits(t *testing.T, store DinerStore) {
	t.Helper()
	daniel, jen, _, _ := storePeople(t, store)
	createYearTestVisit(t, store, "Old Haunt", time.Date(2024, time.June, 10, 18, 0, 0, 0, time.UTC), &daniel,
		map[uuid.UUID]float64{daniel.ID: 8, jen.ID: 8})

	review, err := store.YearInReview(context.Background(), 2023)
	if err != nil {
		t.Fatal(err)
	}
	if review.Year != 2023 || review.TotalVisits != 0 || review.MonthlyVisits != [12]int{} ||
		review.NewRestaurants != 0 || review.RepeatRestaurants != 0 ||
		review.MostVisited != nil || review.HighestRated != nil || len(review.TopPicks) != 0 {
		t.Fatalf("2023 = %#v, want an empty year", review)
	}
}
