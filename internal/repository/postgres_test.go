package repository

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	if err := ensureTestPgcrypto(ctx, admin); err != nil {
		admin.Close(ctx)
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

// ensureTestPgcrypto installs pgcrypto once in public so migration 001's
// CREATE EXTENSION IF NOT EXISTS is a no-op. Otherwise test packages running
// in parallel race to create it in their own schemas. The lock key matches
// the migrate package's tests.
func ensureTestPgcrypto(ctx context.Context, conn *pgx.Conn) error {
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('dined_test_pgcrypto'))"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pgcrypto SCHEMA public")
		return err
	})
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

func TestPostgresPing(t *testing.T) {
	store := postgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := store.Ping(ctx); err != nil {
		t.Fatalf("ping open pool: %v", err)
	}
	store.pool.Close()
	if err := store.Ping(ctx); err == nil {
		t.Fatal("expected ping on a closed pool to fail")
	}
}

func TestPostgresRestaurantsByPlaceIDs(t *testing.T) {
	assertRestaurantsByPlaceIDs(t, postgresStore(t))
}

func TestPostgresAllowsBackToBackPicks(t *testing.T) {
	assertBackToBackPicks(t, postgresStore(t))
}

func TestPostgresEverybodyPicks(t *testing.T) {
	assertEverybodyPicks(t, postgresStore(t))
}

func TestPostgresVisitFilters(t *testing.T) {
	assertVisitFilters(t, postgresStore(t))
}

func TestPostgresNoVisitFilterOptionsWithoutVisits(t *testing.T) {
	assertNoVisitFilterOptionsWithoutVisits(t, postgresStore(t))
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

func TestPostgresTrophyRecords(t *testing.T) {
	assertTrophyRecords(t, postgresStore(t))
}

func TestPostgresYearInReview(t *testing.T) {
	assertYearInReview(t, postgresStore(t))
}

func TestPostgresYearInReviewTiesBreakByRestaurantID(t *testing.T) {
	assertYearInReviewTiesBreakByRestaurantID(t, postgresStore(t))
}

func TestPostgresYearInReviewWithoutVisits(t *testing.T) {
	assertYearInReviewWithoutVisits(t, postgresStore(t))
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
