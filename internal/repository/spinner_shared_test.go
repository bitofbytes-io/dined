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
// Postgres spinner rules. Each expects a store with the four seeded people
// and no visits.

func TestMemoryStoreRestedFavorites(t *testing.T) {
	assertRestedFavorites(t, emptyMemoryStore())
}

func TestPostgresRestedFavorites(t *testing.T) {
	assertRestedFavorites(t, postgresStore(t))
}

func TestMemoryStorePickCounts(t *testing.T) {
	assertPickCounts(t, emptyMemoryStore())
}

func TestPostgresPickCounts(t *testing.T) {
	assertPickCounts(t, postgresStore(t))
}

var spinTestNow = time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)

func createSpinTestVisit(t *testing.T, store DinerStore, name string, daysAgo int, picker *model.Person, ratings map[uuid.UUID]float64) {
	t.Helper()
	input := model.VisitInput{
		RestaurantName: name,
		Address:        "1 " + name + " Way", // Same name and address: the same restaurant.
		Category:       "Diner",
		VisitedAt:      spinTestNow.AddDate(0, 0, -daysAgo),
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

func assertRestedFavorites(t *testing.T, store DinerStore) {
	t.Helper()
	daniel, jen, _, _ := storePeople(t, store)
	two := func(a, b float64) map[uuid.UUID]float64 { return map[uuid.UUID]float64{daniel.ID: a, jen.ID: b} }
	createSpinTestVisit(t, store, "Old Favorite", 60, &daniel, two(8, 8))
	// Exactly the minimum average counts.
	createSpinTestVisit(t, store, "Borderline", 45, &jen, two(6.5, 7.5))
	// A single-rater visit is left out of the average but still counts as a visit.
	createSpinTestVisit(t, store, "Mixed", 80, nil, two(9, 9))
	createSpinTestVisit(t, store, "Mixed", 50, nil, map[uuid.UUID]float64{daniel.ID: 2})
	// Too recent, too low, or never rated by two people.
	createSpinTestVisit(t, store, "Recent Favorite", 60, nil, two(9, 9))
	createSpinTestVisit(t, store, "Recent Favorite", 5, nil, map[uuid.UUID]float64{daniel.ID: 9})
	createSpinTestVisit(t, store, "Meh Place", 60, nil, two(6, 6.5))
	createSpinTestVisit(t, store, "Solo Rated", 60, nil, map[uuid.UUID]float64{daniel.ID: 10})

	favorites, err := store.RestedFavorites(context.Background(), 7, spinTestNow.AddDate(0, 0, -30))
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		name    string
		average float64
		daysAgo int
	}{
		{"Borderline", 7, 45},
		{"Mixed", 9, 50},
		{"Old Favorite", 8, 60},
	}
	if len(favorites) != len(want) {
		t.Fatalf("favorites = %#v, want %v", favorites, want)
	}
	for i, w := range want {
		got := favorites[i]
		if got.Restaurant.Name != w.name || math.Abs(got.AverageRating-w.average) > 0.001 || !got.LastVisitedAt.Equal(spinTestNow.AddDate(0, 0, -w.daysAgo)) {
			t.Fatalf("favorite %d = %s %.2f %s, want %s %.2f %d days ago", i, got.Restaurant.Name, got.AverageRating, got.LastVisitedAt, w.name, w.average, w.daysAgo)
		}
		if got.Restaurant.Category == nil || *got.Restaurant.Category != "Diner" {
			t.Fatalf("favorite %s category = %v, want Diner", got.Restaurant.Name, got.Restaurant.Category)
		}
	}
}

func assertPickCounts(t *testing.T, store DinerStore) {
	t.Helper()
	daniel, jen, caleb, aiden := storePeople(t, store)
	rating := func() map[uuid.UUID]float64 { return map[uuid.UUID]float64{daniel.ID: 8} }
	createSpinTestVisit(t, store, "Daniel One", 10, &daniel, rating())
	createSpinTestVisit(t, store, "Daniel Two", 20, &daniel, rating())
	createSpinTestVisit(t, store, "Jen Long Ago", 100, &jen, rating())
	createSpinTestVisit(t, store, "Aiden One", 5, &aiden, rating())
	createSpinTestVisit(t, store, "Everybody", 1, nil, rating())

	counts, err := store.PickCounts(context.Background(), spinTestNow.AddDate(0, 0, -90))
	if err != nil {
		t.Fatal(err)
	}
	daysAgo := func(days int) *time.Time {
		at := spinTestNow.AddDate(0, 0, -days)
		return &at
	}
	want := []struct {
		person model.Person
		picks  int
		last   *time.Time
	}{
		{daniel, 2, daysAgo(10)},
		{jen, 0, daysAgo(100)},
		{caleb, 0, nil},
		{aiden, 1, daysAgo(5)},
	}
	if len(counts) != len(want) {
		t.Fatalf("counts = %#v, want 4 people", counts)
	}
	for i, w := range want {
		got := counts[i]
		lastOK := (got.LastPickedAt == nil) == (w.last == nil) && (w.last == nil || got.LastPickedAt.Equal(*w.last))
		if got.Person.ID != w.person.ID || got.Person.Name != w.person.Name || got.Picks != w.picks || !lastOK {
			t.Fatalf("count %d = %s %d picks, last %v; want %s %d picks, last %v", i, got.Person.Name, got.Picks, got.LastPickedAt, w.person.Name, w.picks, w.last)
		}
	}
}
