package migrate

import (
	"context"
	"io/fs"
	"os"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/bitofbytes-io/dined/migrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pressly/goose/v3/lock"
)

// postgresSchemaPool creates an isolated schema in the configured test
// database and returns a function that opens pools scoped to it.
func postgresSchemaPool(t *testing.T) func() *pgxpool.Pool {
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
	return func() *pgxpool.Pool {
		t.Helper()
		config, err := pgxpool.ParseConfig(databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
		pool, err := pgxpool.NewWithConfig(context.Background(), config)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(pool.Close)
		return pool
	}
}

func appliedVersions(t *testing.T, pool *pgxpool.Pool) []int64 {
	t.Helper()
	rows, err := pool.Query(context.Background(), "SELECT version_id FROM goose_db_version WHERE version_id > 0 AND is_applied ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	versions, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		t.Fatal(err)
	}
	return versions
}

func embeddedVersionCount(t *testing.T) int {
	t.Helper()
	files, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("find embedded migrations: %v", err)
	}
	return len(files)
}

func TestApplyMigratesFreshDatabaseOnceAndIsIdempotent(t *testing.T) {
	pool := postgresSchemaPool(t)()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	want := embeddedVersionCount(t)
	if got := appliedVersions(t, pool); len(got) != want {
		t.Fatalf("applied versions after first run = %v, want %d versions", got, want)
	}
	var tables int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name IN ('restaurants', 'dining_visits', 'want_to_try')").Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 3 {
		t.Fatalf("expected core tables to exist, found %d of 3", tables)
	}

	if err := Apply(ctx, pool); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if got := appliedVersions(t, pool); len(got) != want {
		t.Fatalf("second run re-applied migrations: versions = %v", got)
	}
}

func TestApplyAppliesOnlyPendingMigrations(t *testing.T) {
	pool := postgresSchemaPool(t)()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	first := fstest.MapFS{
		"001_create.sql": {Data: []byte("-- +goose Up\nCREATE TABLE migrate_probe (id INT);\n-- +goose Down\nDROP TABLE migrate_probe;\n")},
	}
	if err := apply(ctx, pool, first); err != nil {
		t.Fatalf("apply 001: %v", err)
	}

	// Re-running 001 would fail because the table exists, so success proves it
	// was skipped.
	second := fstest.MapFS{
		"001_create.sql": first["001_create.sql"],
		"002_noop.sql":   {Data: []byte("-- +goose Up\nSELECT 1;\n-- +goose Down\nSELECT 1;\n")},
	}
	if err := apply(ctx, pool, second); err != nil {
		t.Fatalf("apply 002: %v", err)
	}
	if got := appliedVersions(t, pool); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("applied versions = %v, want [1 2]", got)
	}
}

func TestApplyFailsOnBrokenMigrationWithoutRecordingIt(t *testing.T) {
	pool := postgresSchemaPool(t)()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fsys := fstest.MapFS{
		"001_create.sql": {Data: []byte("-- +goose Up\nCREATE TABLE migrate_probe (id INT);\n")},
		"002_broken.sql": {Data: []byte("-- +goose Up\nALTER TABLE migrate_probe ADD COLUMN note TEXT;\nSELECT * FROM missing_table;\n")},
	}
	if err := apply(ctx, pool, fsys); err == nil {
		t.Fatal("expected broken migration to fail")
	}
	if got := appliedVersions(t, pool); len(got) != 1 || got[0] != 1 {
		t.Fatalf("applied versions = %v, want [1]", got)
	}
	var columns int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'migrate_probe' AND column_name = 'note'").Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if columns != 0 {
		t.Fatal("broken migration was not rolled back")
	}
}

func TestApplyWaitsForAdvisoryLock(t *testing.T) {
	openPool := postgresSchemaPool(t)
	pool := openPool()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Hold goose's lock the way another starting replica would.
	holder, err := openPool().Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	if _, err := holder.Exec(ctx, "SELECT pg_advisory_lock($1)", lock.DefaultLockID); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- Apply(ctx, pool) }()
	select {
	case err := <-done:
		t.Fatalf("Apply finished while the migration lock was held: %v", err)
	case <-time.After(1500 * time.Millisecond):
	}

	if _, err := holder.Exec(ctx, "SELECT pg_advisory_unlock($1)", lock.DefaultLockID); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("apply after unlock: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("Apply did not finish after the migration lock was released")
	}
	if got, want := appliedVersions(t, pool), embeddedVersionCount(t); len(got) != want {
		t.Fatalf("applied versions = %v, want %d versions", got, want)
	}
}

func TestApplyConcurrentStartupsApplyEachMigrationOnce(t *testing.T) {
	openPool := postgresSchemaPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pools := []*pgxpool.Pool{openPool(), openPool(), openPool()}
	errs := make([]error, len(pools))
	var wg sync.WaitGroup
	for i, pool := range pools {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = Apply(ctx, pool)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
	}
	if got, want := appliedVersions(t, pools[0]), embeddedVersionCount(t); len(got) != want {
		t.Fatalf("applied versions = %v, want %d versions", got, want)
	}
}
