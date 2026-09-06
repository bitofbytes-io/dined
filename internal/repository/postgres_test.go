package repository

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
			return store.RestaurantVisitSummaries(ctx, restaurantID)
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
			_, err = store.scanVisits(ctx, rows, true)
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
