package repository

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Each test gets an isolated schema in an explicitly configured test database.
func postgresStore(t *testing.T) *Store {
	t.Helper()
	return postgresStoreWithTracer(t, nil)
}

func postgresStoreWithTracer(t *testing.T, tracer pgx.QueryTracer) *Store {
	t.Helper()
	databaseURL := os.Getenv("DINED_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set DINED_TEST_DATABASE_URL to run PostgreSQL tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := "dined_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close(ctx)
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		defer admin.Close(ctx)
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	config.ConnConfig.Tracer = tracer
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	migrations, err := filepath.Glob("../../migrations/*.sql")
	if err != nil || len(migrations) == 0 {
		t.Fatalf("find migrations: %v", err)
	}
	for _, migration := range migrations {
		content, err := os.ReadFile(migration)
		if err != nil {
			t.Fatal(err)
		}
		up, _, _ := strings.Cut(string(content), "-- +goose Down")
		if _, err := pool.Exec(ctx, up); err != nil {
			t.Fatalf("apply %s: %v", migration, err)
		}
	}
	return New(pool)
}

func createPostgresVisit(t *testing.T, store *Store) (uuid.UUID, uuid.UUID) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tags, err := store.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Single connection diner",
		VisitedAt:      time.Now(), PickerID: &people[0].ID, PriceLevel: 2,
		Ratings: map[uuid.UUID]float64{people[0].ID: 8.5}, TagIDs: []uuid.UUID{tags[0].ID},
		Photos: []model.VisitPhotoInput{{DataURI: testVisitPhotoDataURI}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var restaurantID uuid.UUID
	if err := store.pool.QueryRow(ctx, "SELECT restaurant_id FROM dining_visits WHERE id=$1", id).Scan(&restaurantID); err != nil {
		t.Fatal(err)
	}
	return *id, restaurantID
}

func TestPostgresVisitReadsReleaseBaseConnection(t *testing.T) {
	store := postgresStore(t)
	visitID, restaurantID := createPostgresVisit(t, store)
	tests := []struct {
		name   string
		read   func(context.Context) ([]model.Visit, error)
		photos int
	}{
		{"visit", func(ctx context.Context) ([]model.Visit, error) {
			visit, err := store.Visit(ctx, visitID)
			if err != nil || visit == nil {
				return nil, err
			}
			return []model.Visit{*visit}, nil
		}, 1},
		{"visits", func(ctx context.Context) ([]model.Visit, error) { return store.Visits(ctx, 10) }, 1},
		{"restaurant visits", func(ctx context.Context) ([]model.Visit, error) { return store.RestaurantVisits(ctx, restaurantID) }, 1},
		{"restaurant summaries", func(ctx context.Context) ([]model.Visit, error) {
			summaries, err := store.RestaurantVisitSummaries(ctx, []uuid.UUID{restaurantID})
			return summaries[restaurantID], err
		}, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			visits, err := test.read(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(visits) != 1 {
				t.Fatalf("got %d visits", len(visits))
			}
			visit := visits[0]
			if visit.ID != visitID || len(visit.Ratings) != 1 || visit.Ratings[0].Score != 8.5 || len(visit.Tags) != 1 || len(visit.Photos) != test.photos {
				t.Fatalf("incorrect related records: %#v", visit)
			}
		})
	}
	t.Run("concurrent reads", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var wg sync.WaitGroup
		for range 12 {
			wg.Go(func() {
				visits, err := store.Visits(ctx, 0)
				if err != nil {
					t.Error(err)
					return
				}
				if len(visits) != 1 {
					t.Errorf("got %d visits", len(visits))
				}
			})
		}
		wg.Wait()
	})
}

func TestPostgresVisitReadErrorsReleaseConnection(t *testing.T) {
	store := postgresStore(t)
	createPostgresVisit(t, store)
	for _, query := range []string{
		"SELECT 1", // Scan has the wrong number of columns.
		visitSelectSQL() + " WHERE 1 / (length(r.name) - length(r.name)) > 0", // Server-side row iteration error.
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		rows, err := store.pool.Query(ctx, query)
		if err == nil {
			_, err = store.scanVisits(ctx, rows, withPhotoData)
		}
		if err == nil {
			t.Errorf("expected error from %q", query)
		}
		if err := store.pool.Ping(ctx); err != nil {
			t.Errorf("connection not reusable: %v", err)
		}
		cancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := store.pool.Exec(ctx, "DROP TABLE visit_tags"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Visits(ctx, 0); err == nil {
		t.Fatal("expected related query error")
	}
	if err := store.pool.Ping(ctx); err != nil {
		t.Fatalf("connection not reusable after related query failure: %v", err)
	}
}

// queryCounter counts queries sent through a pool so tests can assert batching.
type queryCounter struct {
	count atomic.Int64
}

func (c *queryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.count.Add(1)
	return ctx
}

func (c *queryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (c *queryCounter) queriesDuring(t *testing.T, read func() error) int64 {
	t.Helper()
	before := c.count.Load()
	if err := read(); err != nil {
		t.Fatal(err)
	}
	return c.count.Load() - before
}

func TestPostgresVisitListsBatchRelatedQueries(t *testing.T) {
	counter := &queryCounter{}
	store := postgresStoreWithTracer(t, counter)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tags, err := store.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const visitCount = 5
	var restaurantIDs []uuid.UUID
	base := time.Now().Add(-time.Hour)
	for i := range visitCount {
		id, err := store.CreateVisit(ctx, model.VisitInput{
			RestaurantName: "Batch diner " + strconv.Itoa(i),
			VisitedAt:      base.Add(time.Duration(i) * time.Minute),
			PickerID:       &people[0].ID,
			PriceLevel:     2,
			Ratings:        map[uuid.UUID]float64{people[0].ID: float64(i), people[1].ID: 9},
			TagIDs:         []uuid.UUID{tags[i%len(tags)].ID},
			Photos:         []model.VisitPhotoInput{{DataURI: testVisitPhotoDataURI}},
		})
		if err != nil {
			t.Fatal(err)
		}
		visit, err := store.Visit(ctx, *id)
		if err != nil {
			t.Fatal(err)
		}
		restaurantIDs = append(restaurantIDs, visit.Restaurant.ID)
	}

	var visits []model.Visit
	queries := counter.queriesDuring(t, func() (err error) {
		visits, err = store.VisitsPage(ctx, 20, 0)
		return err
	})
	// Base visit query plus one query each for ratings, tags, and photos.
	if queries != 4 {
		t.Fatalf("visit list used %d queries, want 4", queries)
	}
	if len(visits) != visitCount {
		t.Fatalf("got %d visits, want %d", len(visits), visitCount)
	}
	for i, visit := range visits {
		wantScore := float64(visitCount - 1 - i) // Newest first.
		if len(visit.Ratings) != 2 || visit.Ratings[0].Person.ID != people[0].ID || visit.Ratings[0].Score != wantScore {
			t.Fatalf("visit %d ratings = %#v", i, visit.Ratings)
		}
		if len(visit.Tags) != 1 || visit.Tags[0].ID != tags[(visitCount-1-i)%len(tags)].ID {
			t.Fatalf("visit %d tags = %#v", i, visit.Tags)
		}
		if len(visit.Photos) != 1 || visit.Photos[0].VisitID != visit.ID || visit.Photos[0].DataURI != "" || visit.Photos[0].ByteCount != 5 {
			t.Fatalf("visit %d photos should carry metadata only: %#v", i, visit.Photos)
		}
	}

	photo, err := store.VisitPhoto(ctx, visits[0].Photos[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if photo == nil || photo.DataURI != testVisitPhotoDataURI {
		t.Fatalf("visit photo = %#v", photo)
	}
	if missing, err := store.VisitPhoto(ctx, uuid.New()); err != nil || missing != nil {
		t.Fatalf("missing photo = %#v, err = %v", missing, err)
	}

	var summaries map[uuid.UUID][]model.Visit
	queries = counter.queriesDuring(t, func() (err error) {
		summaries, err = store.RestaurantVisitSummaries(ctx, restaurantIDs)
		return err
	})
	// Base visit query plus ratings and tags; summaries skip photos.
	if queries != 3 {
		t.Fatalf("restaurant summaries used %d queries, want 3", queries)
	}
	for _, id := range restaurantIDs {
		if got := summaries[id]; len(got) != 1 || len(got[0].Ratings) != 2 || len(got[0].Tags) != 1 || len(got[0].Photos) != 0 {
			t.Fatalf("summaries for %s = %#v", id, got)
		}
	}
	if empty, err := store.RestaurantVisitSummaries(ctx, nil); err != nil || len(empty) != 0 {
		t.Fatalf("empty summaries = %#v, err = %v", empty, err)
	}
}

func TestPostgresVisitsPageOffsets(t *testing.T) {
	store := postgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ids []uuid.UUID
	base := time.Now().Add(-time.Hour)
	for i := range 5 {
		id, err := store.CreateVisit(ctx, model.VisitInput{
			RestaurantName: "Page diner " + strconv.Itoa(i),
			VisitedAt:      base.Add(-time.Duration(i) * time.Minute),
			PickerID:       &people[0].ID,
			PriceLevel:     2,
			Ratings:        map[uuid.UUID]float64{people[0].ID: 7},
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, *id)
	}
	tests := []struct {
		limit, offset int
		want          []uuid.UUID
	}{
		{limit: 2, offset: 0, want: ids[:2]},
		{limit: 2, offset: 2, want: ids[2:4]},
		{limit: 2, offset: 4, want: ids[4:]},
		{limit: 2, offset: 5, want: nil},
		{limit: 0, offset: 3, want: ids[3:]},
	}
	for _, test := range tests {
		visits, err := store.VisitsPage(ctx, test.limit, test.offset)
		if err != nil {
			t.Fatal(err)
		}
		var got []uuid.UUID
		for _, visit := range visits {
			got = append(got, visit.ID)
		}
		if !slices.Equal(got, test.want) {
			t.Fatalf("VisitsPage(%d, %d) = %v, want %v", test.limit, test.offset, got, test.want)
		}
	}
}

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

func TestPostgresUpdateVisitReplacesRatingsAndTags(t *testing.T) {
	store := postgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tags, err := store.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}

	input := postgresVisitInput(t, store, "Tag Diner")
	input.Ratings = map[uuid.UUID]float64{people[0].ID: 8, people[1].ID: 6.5}
	input.TagIDs = []uuid.UUID{tags[0].ID}
	input.NewTag = "Patio"
	visitID, err := store.CreateVisit(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Ratings) != 2 || len(created.Tags) != 2 {
		t.Fatalf("created ratings = %#v, tags = %#v", created.Ratings, created.Tags)
	}

	update := postgresVisitInput(t, store, "")
	update.RestaurantID = &created.Restaurant.ID
	update.Ratings = map[uuid.UUID]float64{people[1].ID: 9.5}
	update.TagIDs = []uuid.UUID{tags[1].ID}
	update.NewTag = "Late Night"
	if err := store.UpdateVisit(ctx, *visitID, update); err != nil {
		t.Fatal(err)
	}

	updated, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Ratings) != 1 || updated.Ratings[0].Person.ID != people[1].ID || updated.Ratings[0].Score != 9.5 {
		t.Fatalf("updated ratings = %#v", updated.Ratings)
	}
	var tagNames []string
	for _, tag := range updated.Tags {
		tagNames = append(tagNames, tag.Name)
	}
	if want := []string{"Late Night", tags[1].Name}; !slices.Equal(tagNames, want) && !slices.Equal(tagNames, []string{want[1], want[0]}) {
		t.Fatalf("updated tags = %v, want %v", tagNames, want)
	}
}

func TestPostgresUpdateVisitReconcilesPhotos(t *testing.T) {
	store := postgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	input := postgresVisitInput(t, store, "Photo Diner")
	input.Photos = []model.VisitPhotoInput{
		{DataURI: "data:image/jpeg;base64,b25l"},
		{DataURI: "data:image/jpeg;base64,dHdv"},
		{DataURI: "data:image/jpeg;base64,dGhyZWU="},
	}
	visitID, err := store.CreateVisit(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(created.Photos) != 3 {
		t.Fatalf("created photos = %d, want 3", len(created.Photos))
	}
	one, two, three := created.Photos[0], created.Photos[1], created.Photos[2]

	update := postgresVisitInput(t, store, "")
	update.RestaurantID = &created.Restaurant.ID
	update.KeepPhotoIDs = []uuid.UUID{three.ID, one.ID}
	update.Photos = []model.VisitPhotoInput{{DataURI: "data:image/jpeg;base64,Zm91cg=="}}
	if err := store.UpdateVisit(ctx, *visitID, update); err != nil {
		t.Fatal(err)
	}

	updated, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Photos) != 3 {
		t.Fatalf("updated photos = %#v", updated.Photos)
	}
	for i, want := range []model.VisitPhoto{three, one} {
		got := updated.Photos[i]
		if got.ID != want.ID || got.DataURI != want.DataURI || got.SortOrder != i || !got.CreatedAt.Equal(want.CreatedAt) {
			t.Fatalf("kept photo %d = %#v, want %#v at sort order %d", i, got, want, i)
		}
	}
	added := updated.Photos[2]
	if added.ID == one.ID || added.ID == two.ID || added.ID == three.ID || added.DataURI != "data:image/jpeg;base64,Zm91cg==" || added.SortOrder != 2 || added.ByteCount != 4 {
		t.Fatalf("new photo = %#v", added)
	}
	if removed, err := store.VisitPhoto(ctx, two.ID); err != nil || removed != nil {
		t.Fatalf("removed photo = %#v, err = %v", removed, err)
	}

	update.KeepPhotoIDs = []uuid.UUID{two.ID}
	update.Photos = nil
	if err := store.UpdateVisit(ctx, *visitID, update); err == nil || !strings.Contains(err.Error(), "photo not found") {
		t.Fatalf("keeping a photo from outside the visit error = %v", err)
	}
	unchanged, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(unchanged.Photos) != 3 || unchanged.Photos[0].ID != three.ID {
		t.Fatalf("failed update should roll back photos: %#v", unchanged.Photos)
	}
}

func TestPostgresDeleteVisitCascadesAndFreesRestaurant(t *testing.T) {
	store := postgresStore(t)
	visitID, restaurantID := createPostgresVisit(t, store)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	deleted, err := store.DeleteRestaurantIfUnvisited(ctx, restaurantID)
	if err != nil {
		t.Fatal(err)
	}
	if deleted {
		t.Fatal("restaurant with a visit should not be deleted")
	}

	if err := store.DeleteVisit(ctx, visitID); err != nil {
		t.Fatal(err)
	}
	if visit, err := store.Visit(ctx, visitID); err != nil || visit != nil {
		t.Fatalf("deleted visit = %#v, err = %v", visit, err)
	}
	for _, table := range []string{"visit_participant_ratings", "visit_tags", "visit_photos"} {
		if count := countPostgresRows(t, store, "SELECT COUNT(*) FROM "+table+" WHERE visit_id = $1", visitID); count != 0 {
			t.Fatalf("%s rows after delete = %d, want 0", table, count)
		}
	}

	deleted, err = store.DeleteRestaurantIfUnvisited(ctx, restaurantID)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("restaurant without visits should be deleted")
	}
	if restaurant, err := store.Restaurant(ctx, restaurantID); err != nil || restaurant != nil {
		t.Fatalf("deleted restaurant = %#v, err = %v", restaurant, err)
	}
}

func TestPostgresVisitPositionMatchesVisitsPageOrder(t *testing.T) {
	store := postgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sameTime := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	for i := range 6 {
		input := postgresVisitInput(t, store, "Position diner "+strconv.Itoa(i))
		input.VisitedAt = sameTime.Add(-time.Duration(i%3) * time.Minute) // Pairs share a visit time.
		if _, err := store.CreateVisit(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	// Force a full tie on visited_at and created_at so the id tie-breaker decides.
	if _, err := store.pool.Exec(ctx, "UPDATE dining_visits SET created_at = $1 WHERE visited_at = $2", sameTime, sameTime); err != nil {
		t.Fatal(err)
	}

	visits, err := store.VisitsPage(ctx, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	for want, visit := range visits {
		got, found, err := store.VisitPosition(ctx, visit.ID)
		if err != nil || !found || got != want {
			t.Fatalf("VisitPosition(%s) = %d, %v, %v; want %d", visit.ID, got, found, err, want)
		}
	}
	if _, found, err := store.VisitPosition(ctx, uuid.New()); err != nil || found {
		t.Fatalf("missing visit found = %v, err = %v", found, err)
	}
}

func TestPostgresAllowsBackToBackPicks(t *testing.T) {
	assertBackToBackPicks(t, postgresStore(t))
}

func TestPostgresEverybodyPicks(t *testing.T) {
	assertEverybodyPicks(t, postgresStore(t))
}

func TestPostgresRejectsUnknownPicker(t *testing.T) {
	assertUnknownPickerRejected(t, postgresStore(t))
}

func TestPostgresAggregatesNeedTwoRaters(t *testing.T) {
	assertAggregatesNeedTwoRaters(t, postgresStore(t))
}

func TestPostgresNoAggregatesWithoutTwoRaters(t *testing.T) {
	assertNoAggregatesWithoutTwoRaters(t, postgresStore(t))
}

func TestPostgresRejectsPlaceIDOwnedByAnotherRestaurant(t *testing.T) {
	assertChosenRestaurantPlaceOwnedElsewhereRejected(t, postgresStore(t))
}

func TestPostgresRejectsDifferentPlaceForLinkedRestaurant(t *testing.T) {
	assertChosenRestaurantDifferentPlaceRejected(t, postgresStore(t))
}

func TestPostgresAcceptsMatchingPlaceForChosenRestaurant(t *testing.T) {
	assertChosenRestaurantMatchingPlaceAccepted(t, postgresStore(t))
}

func TestPostgresEditVisitIgnoresPlaceID(t *testing.T) {
	assertEditVisitIgnoresPlaceID(t, postgresStore(t))
}

func TestPostgresIgnoresPlaceDetailsWithoutPlaceID(t *testing.T) {
	assertChosenRestaurantIgnoresDetailsWithoutPlaceID(t, postgresStore(t))
}
