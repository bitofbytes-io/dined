package repository

import (
	"context"
	"math"
	"testing"

	"github.com/google/uuid"
)

// These checks run against both stores so the memory preview keeps the
// Postgres rules for the "Haven't been in a while" list. They reuse the
// spinner tests' helpers and expect a store with no visits.

func TestMemoryStorePastFavorites(t *testing.T) {
	assertPastFavorites(t, emptyMemoryStore())
}

func TestPostgresPastFavorites(t *testing.T) {
	assertPastFavorites(t, postgresStore(t))
}

func assertPastFavorites(t *testing.T, store DinerStore) {
	t.Helper()
	daniel, jen, _, _ := storePeople(t, store)
	two := func(a, b float64) map[uuid.UUID]float64 { return map[uuid.UUID]float64{daniel.ID: a, jen.ID: b} }
	// Best rated first, then the longest since the last visit.
	createSpinTestVisit(t, store, "Top Rated", 200, nil, two(9, 9))
	createSpinTestVisit(t, store, "Older Good", 400, nil, two(8, 8))
	createSpinTestVisit(t, store, "Newer Good", 250, nil, two(8, 8))
	// Exactly the minimum average and old enough: kept.
	createSpinTestVisit(t, store, "Borderline", 300, nil, two(6.5, 7.5))
	// A later visit, rated by one person or not, makes a place recent.
	createSpinTestVisit(t, store, "Revisited", 300, nil, two(9, 9))
	createSpinTestVisit(t, store, "Revisited", 20, nil, map[uuid.UUID]float64{daniel.ID: 5})
	// Too recent, too low, or never rated by two people.
	createSpinTestVisit(t, store, "Recent Favorite", 100, nil, two(9, 9))
	createSpinTestVisit(t, store, "Meh Place", 300, nil, two(6, 6.5))
	createSpinTestVisit(t, store, "Solo Rated", 300, nil, map[uuid.UUID]float64{daniel.ID: 10})

	const minAverage, months = 7.0, 6
	favorites, err := store.PastFavorites(context.Background(), minAverage, spinTestNow.AddDate(0, -months, 0))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		name    string
		average float64
		daysAgo int
	}{
		{"Top Rated", 9, 200},
		{"Older Good", 8, 400},
		{"Newer Good", 8, 250},
		{"Borderline", 7, 300},
	}
	if len(favorites) != len(want) {
		t.Fatalf("past favorites = %#v, want %v", favorites, want)
	}
	for i, w := range want {
		got := favorites[i]
		if got.Restaurant.Name != w.name || math.Abs(got.AverageRating-w.average) > 0.001 || !got.LastVisitedAt.Equal(spinTestNow.AddDate(0, 0, -w.daysAgo)) {
			t.Fatalf("past favorite %d = %s %.2f %s, want %s %.2f %d days ago", i, got.Restaurant.Name, got.AverageRating, got.LastVisitedAt, w.name, w.average, w.daysAgo)
		}
	}

	// Closed restaurants stay off the list.
	closed := favorites[0].Restaurant
	setRestaurantClosed(t, store, closed.ID, true)
	favorites, err = store.PastFavorites(context.Background(), minAverage, spinTestNow.AddDate(0, -months, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(favorites) != len(want)-1 || favorites[0].Restaurant.ID == closed.ID {
		t.Fatalf("past favorites after closing %s = %#v", closed.Name, favorites)
	}
}
