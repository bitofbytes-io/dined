package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestDatabasePoolConfigReadsOnlyDatabaseSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database")
	if err := os.WriteFile(path, []byte("postgres://fixture:fixture-password@localhost/disposable"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DATABASE_URL_FILE", path)
	// These deliberately unreadable paths must never be consulted by this tool.
	t.Setenv("AUTH_GOOGLE_CLIENT_ID_FILE", t.TempDir())
	t.Setenv("AUTH_GOOGLE_CLIENT_SECRET_FILE", t.TempDir())
	t.Setenv("GOOGLE_PLACES_API_KEY_FILE", t.TempDir())
	cfg, err := databasePoolConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.Database != "disposable" || cfg.ConnConfig.Password != "fixture-password" || cfg.MaxConns != 1 {
		t.Fatal("wrong database configuration")
	}
	t.Setenv("DATABASE_URL", "postgres://environment@localhost/env-disposable")
	cfg, err = databasePoolConfig()
	if err != nil || cfg.ConnConfig.Database != "env-disposable" {
		t.Fatal("environment precedence changed", err)
	}
}

func TestDatabaseConfigurationFailuresAreRedacted(t *testing.T) {
	for _, value := range []string{"postgres://fixture:secret-value@invalid host/disposable", "host=localhost password='secret-value"} {
		t.Setenv("DATABASE_URL", value)
		_, err := databasePoolConfig()
		if err == nil || strings.Contains(err.Error(), "secret-value") || err.Error() != "invalid database configuration" {
			t.Fatal("configuration failure was not redacted")
		}
	}
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DATABASE_URL_FILE", t.TempDir())
	_, err := databasePoolConfig()
	if err == nil || err.Error() != "cannot read database configuration" {
		t.Fatal("secret file failure was not redacted")
	}
}

func TestDatabaseQueryErrorsDoNotExposeRowValues(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: "23514", Message: "secret-value", Detail: "private-photo-data"})
	if got := safeDatabaseError(err).Error(); got != "database operation failed (SQLSTATE 23514)" {
		t.Fatal("database row details were exposed")
	}
	// Connection errors are tested through the real driver's dial path with a
	// controlled local-only DSN, never production credentials.
	cfg, err := pgconn.ParseConfig("postgres://fixture:secret-value@127.0.0.1:1/disposable?connect_timeout=1")
	if err != nil {
		t.Fatal(err)
	}
	cfg.DialFunc = func(context.Context, string, string) (net.Conn, error) {
		return nil, fmt.Errorf("fixture dial error secret-value")
	}
	_, err = pgconn.ConnectConfig(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected connection failure")
	}
	if got := safeDatabaseError(err).Error(); got != "database connection failed" {
		t.Fatal("connection details were exposed")
	}
}
