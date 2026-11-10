package main

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/auth"
	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestSlogLevel(t *testing.T) {
	tests := []struct {
		level string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"verbose", slog.LevelInfo},
		// Matching is exact: config is expected to normalize case.
		{"DEBUG", slog.LevelInfo},
		{"Warn", slog.LevelInfo},
		{" error ", slog.LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			if got := slogLevel(tt.level); got != tt.want {
				t.Fatalf("slogLevel(%q) = %v, want %v", tt.level, got, tt.want)
			}
		})
	}
}

func TestOpenStoresMemory(t *testing.T) {
	// The memory store must not touch the database URL.
	cfg := &config.Config{DataStore: config.DataStoreMemory, DatabaseURL: "postgres://invalid.invalid:1/none"}

	store, authRepo, pool, closeStores, err := openStores(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.(*repository.MemoryStore); !ok {
		t.Fatalf("store = %T, want *repository.MemoryStore", store)
	}
	if _, ok := authRepo.(*auth.MemoryRepository); !ok {
		t.Fatalf("auth repository = %T, want *auth.MemoryRepository", authRepo)
	}
	if pool != nil {
		t.Fatal("memory store returned a Postgres pool")
	}
	if closeStores == nil {
		t.Fatal("closeStores is nil")
	}
	closeStores()
}

func TestOpenStoresPostgresErrors(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{"malformed url", "postgres://user@host:notaport/db", "connect database"},
		// Port 1 on loopback refuses connections, so Ping fails without a server.
		{"unreachable database", "postgres://dined:secretpw@127.0.0.1:1/dined?sslmode=disable&connect_timeout=1", "ping database"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{DataStore: config.DataStorePostgres, DatabaseURL: tt.url}

			store, authRepo, pool, closeStores, err := openStores(context.Background(), cfg)
			if err == nil {
				closeStores()
				t.Fatal("openStores succeeded, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want it to contain %q", err, tt.wantErr)
			}
			if strings.Contains(err.Error(), "secretpw") {
				t.Fatalf("error leaks credentials: %q", err)
			}
			if store != nil || authRepo != nil || pool != nil || closeStores != nil {
				t.Fatal("openStores returned non-nil values alongside an error")
			}
		})
	}
}

func TestOpenStoresPostgres(t *testing.T) {
	databaseURL := os.Getenv("DINED_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set DINED_TEST_DATABASE_URL to run PostgreSQL tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := pgx.BeginFunc(ctx, admin, func(tx pgx.Tx) error {
		// Install pgcrypto once in public, as the other Postgres tests do, so
		// migration 001's CREATE EXTENSION IF NOT EXISTS is a no-op.
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext('dined_test_pgcrypto'))"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS pgcrypto SCHEMA public")
		return err
	}); err != nil {
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

	// Scope the pool to the isolated schema through a pgx runtime parameter.
	u, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema+",public")
	u.RawQuery = q.Encode()
	cfg := &config.Config{DataStore: config.DataStorePostgres, DatabaseURL: u.String()}

	store, authRepo, pool, closeStores, err := openStores(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			closeStores()
		}
	}()
	if _, ok := store.(*repository.Store); !ok {
		t.Fatalf("store = %T, want *repository.Store", store)
	}
	if _, ok := authRepo.(*auth.PostgresRepository); !ok {
		t.Fatalf("auth repository = %T, want *auth.PostgresRepository", authRepo)
	}
	if pool == nil {
		t.Fatal("Postgres store returned a nil pool")
	}

	// Migrations ran before openStores returned.
	var applied int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+schema+".goose_db_version").Scan(&applied); err != nil {
		t.Fatalf("migrations were not applied: %v", err)
	}
	if applied == 0 {
		t.Fatal("goose_db_version is empty")
	}
	if _, err := store.People(ctx); err != nil {
		t.Fatalf("store query after migration: %v", err)
	}

	closeStores()
	closed = true
	if err := pool.Ping(ctx); err == nil {
		t.Fatal("pool still usable after closeStores")
	}
}
