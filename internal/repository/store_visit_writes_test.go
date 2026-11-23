package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func postgresVisitInput(t *testing.T, store *Store, name string) model.VisitInput {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return model.VisitInput{
		RestaurantName: name,
		VisitedAt:      time.Now().Add(-time.Hour),
		PickerID:       &people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	}
}

func postgresVisitRestaurant(t *testing.T, store *Store, visitID uuid.UUID) model.Restaurant {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	visit, err := store.Visit(ctx, visitID)
	if err != nil {
		t.Fatal(err)
	}
	if visit == nil {
		t.Fatalf("visit %s not found", visitID)
	}
	return visit.Restaurant
}

func countPostgresRows(t *testing.T, store *Store, query string, args ...any) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var count int
	if err := store.pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func TestPostgresCreateVisitMatchesRestaurantByPlaceID(t *testing.T) {
	store := postgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	first := postgresVisitInput(t, store, "Place Diner")
	first.GooglePlaceID = "place-diner"
	first.Category = "American"
	firstID, err := store.CreateVisit(ctx, first)
	if err != nil {
		t.Fatal(err)
	}

	second := postgresVisitInput(t, store, "Place Diner Renamed")
	second.GooglePlaceID = "place-diner"
	second.Address = "12 Oak Street"
	second.City = "Raleigh"
	second.Category = "Diner"
	secondID, err := store.CreateVisit(ctx, second)
	if err != nil {
		t.Fatal(err)
	}

	firstRestaurant := postgresVisitRestaurant(t, store, *firstID)
	secondRestaurant := postgresVisitRestaurant(t, store, *secondID)
	if firstRestaurant.ID != secondRestaurant.ID {
		t.Fatalf("visits should share the place ID restaurant: %s != %s", firstRestaurant.ID, secondRestaurant.ID)
	}
	if secondRestaurant.Name != "Place Diner" || *secondRestaurant.Category != "American" {
		t.Fatalf("existing restaurant values should be preserved: %#v", secondRestaurant)
	}
	if secondRestaurant.Address == nil || *secondRestaurant.Address != "12 Oak Street" || secondRestaurant.City == nil || *secondRestaurant.City != "Raleigh" {
		t.Fatalf("missing restaurant values should be filled: %#v", secondRestaurant)
	}
	if got := countPostgresRows(t, store, "SELECT COUNT(*) FROM restaurants"); got != 1 {
		t.Fatalf("restaurants = %d, want 1", got)
	}
}

func TestPostgresCreateVisitMatchesRestaurantByNameAndAddress(t *testing.T) {
	store := postgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	first := postgresVisitInput(t, store, "Corner Cafe")
	first.Address = "5 Elm Street"
	firstID, err := store.CreateVisit(ctx, first)
	if err != nil {
		t.Fatal(err)
	}

	second := postgresVisitInput(t, store, "  corner cafe ")
	second.Address = "5 ELM STREET"
	second.GooglePlaceID = "place-corner"
	second.Category = "Cafe"
	secondID, err := store.CreateVisit(ctx, second)
	if err != nil {
		t.Fatal(err)
	}

	other := postgresVisitInput(t, store, "Corner Cafe")
	other.Address = "9 Pine Street"
	otherID, err := store.CreateVisit(ctx, other)
	if err != nil {
		t.Fatal(err)
	}

	matched := postgresVisitRestaurant(t, store, *secondID)
	if matched.ID != postgresVisitRestaurant(t, store, *firstID).ID {
		t.Fatal("case-insensitive name and address should reuse the restaurant")
	}
	if matched.Name != "Corner Cafe" || matched.GooglePlaceID == nil || *matched.GooglePlaceID != "place-corner" || matched.Category == nil || *matched.Category != "Cafe" {
		t.Fatalf("matched restaurant should keep its name and gain the place ID and category: %#v", matched)
	}
	if postgresVisitRestaurant(t, store, *otherID).ID == matched.ID {
		t.Fatal("a different address should create a separate restaurant")
	}
}

// A concurrent insert of the same place ID must resolve through ON CONFLICT instead of failing.
func TestPostgresCreateVisitUpsertsConcurrentPlaceIDInsert(t *testing.T) {
	store := postgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	racer, err := pgx.ConnectConfig(ctx, store.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer racer.Close(context.Background())
	tx, err := racer.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	var racerID uuid.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO restaurants (name, google_place_id)
		VALUES ('Racing Diner', 'place-race')
		RETURNING id`).Scan(&racerID); err != nil {
		t.Fatal(err)
	}

	input := postgresVisitInput(t, store, "Racing Diner From Google")
	input.GooglePlaceID = "place-race"
	input.Address = "1 Main Street"
	type result struct {
		id  *uuid.UUID
		err error
	}
	done := make(chan result, 1)
	go func() {
		id, err := store.CreateVisit(ctx, input)
		done <- result{id, err}
	}()

	// The place ID lookup cannot see the uncommitted row, so CreateVisit reaches the
	// INSERT and waits on the unique index until the racing transaction commits.
	observer, err := pgx.ConnectConfig(ctx, store.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int
		if err := observer.QueryRow(ctx, `
			SELECT COUNT(*) FROM pg_stat_activity
			WHERE wait_event_type = 'Lock' AND query LIKE '%INSERT INTO restaurants%'`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("CreateVisit never blocked on the conflicting restaurant insert")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	got := <-done
	if got.err != nil {
		t.Fatal(got.err)
	}
	restaurant := postgresVisitRestaurant(t, store, *got.id)
	if restaurant.ID != racerID {
		t.Fatalf("visit restaurant = %s, want concurrently inserted %s", restaurant.ID, racerID)
	}
	if restaurant.Name != "Racing Diner" || restaurant.Address == nil || *restaurant.Address != "1 Main Street" {
		t.Fatalf("upsert should keep the name and fill missing values: %#v", restaurant)
	}
	if count := countPostgresRows(t, store, "SELECT COUNT(*) FROM restaurants WHERE google_place_id = 'place-race'"); count != 1 {
		t.Fatalf("restaurants with place ID = %d, want 1", count)
	}
}

func TestPostgresCreateVisitFillsChosenRestaurantDetails(t *testing.T) {
	store := postgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	first := postgresVisitInput(t, store, "Chosen Diner")
	first.Category = "American"
	firstID, err := store.CreateVisit(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	restaurant := postgresVisitRestaurant(t, store, *firstID)

	latitude, longitude, rating, priceLevel := 35.78, -78.64, 4.4, 2
	second := postgresVisitInput(t, store, "Ignored Name")
	second.RestaurantID = &restaurant.ID
	second.Address = "9 Elm Street"
	second.City = "Cary"
	second.Category = "Diner"
	second.GooglePlaceID = "chosen-diner"
	second.GoogleMetadata = model.GoogleRestaurantMetadata{
		Latitude: &latitude, Longitude: &longitude, Phone: "919-555-0100",
		Website: "https://chosen.example", GoogleRating: &rating, GooglePriceLevel: &priceLevel,
	}
	secondID, err := store.CreateVisit(ctx, second)
	if err != nil {
		t.Fatal(err)
	}

	filled := postgresVisitRestaurant(t, store, *secondID)
	if filled.ID != restaurant.ID || filled.Name != "Chosen Diner" || filled.Category == nil || *filled.Category != "American" {
		t.Fatalf("chosen restaurant should keep its name and category: %#v", filled)
	}
	if filled.Address == nil || *filled.Address != "9 Elm Street" || filled.City == nil || *filled.City != "Cary" ||
		filled.GooglePlaceID == nil || *filled.GooglePlaceID != "chosen-diner" ||
		filled.Latitude == nil || *filled.Latitude != latitude || filled.Longitude == nil || *filled.Longitude != longitude ||
		filled.Phone == nil || *filled.Phone != "919-555-0100" || filled.Website == nil || *filled.Website != "https://chosen.example" ||
		filled.GoogleRating == nil || *filled.GoogleRating != rating || filled.GooglePriceLevel == nil || *filled.GooglePriceLevel != priceLevel {
		t.Fatalf("missing restaurant details should be filled: %#v", filled)
	}
	if got := countPostgresRows(t, store, "SELECT COUNT(*) FROM restaurants"); got != 1 {
		t.Fatalf("restaurants = %d, want 1", got)
	}
}

// A visit linking a restaurant to one place must not interleave with another
// linking it to a different place: the second sees the first's place ID and is
// rejected, rather than keeping that ID and filling its own place's metadata.
func TestPostgresChosenRestaurantPlaceCheckWaitsForConcurrentLink(t *testing.T) {
	store := postgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	firstID, err := store.CreateVisit(ctx, postgresVisitInput(t, store, "Contested Diner"))
	if err != nil {
		t.Fatal(err)
	}
	chosen := postgresVisitRestaurant(t, store, *firstID)

	racer, err := pgx.ConnectConfig(ctx, store.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer racer.Close(context.Background())
	tx, err := racer.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `UPDATE restaurants SET google_place_id = 'place-first' WHERE id = $1`, chosen.ID); err != nil {
		t.Fatal(err)
	}

	input := postgresVisitInput(t, store, "Contested Diner")
	input.RestaurantID = &chosen.ID
	input.GooglePlaceID = "place-second"
	input.GoogleMetadata = googleOwnedMetadata()
	done := make(chan error, 1)
	go func() {
		_, err := store.CreateVisit(ctx, input)
		done <- err
	}()

	observer, err := pgx.ConnectConfig(ctx, store.pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatal(err)
	}
	defer observer.Close(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	for {
		var waiting int
		if err := observer.QueryRow(ctx, `
			SELECT COUNT(*) FROM pg_stat_activity
			WHERE wait_event_type = 'Lock' AND query LIKE '%restaurants%'`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("CreateVisit never waited on the restaurant being linked")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	var conflict *model.PlaceConflictError
	if err := <-done; !errors.As(err, &conflict) || conflict.Owner != "" {
		t.Fatalf("CreateVisit error = %v, want PlaceConflictError for the now-linked restaurant", err)
	}
	after := postgresVisitRestaurant(t, store, *firstID)
	if after.GooglePlaceID == nil || *after.GooglePlaceID != "place-first" || after.Phone != nil || after.Latitude != nil {
		t.Fatalf("restaurant should keep the first place and gain no other metadata: %#v", after)
	}
}
