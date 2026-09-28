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
		VisitedAt:      time.Now(), PickerID: people[0].ID, PriceLevel: 2,
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
			PickerID:       people[0].ID,
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
			PickerID:       people[0].ID,
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
