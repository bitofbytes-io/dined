// Package migrate applies the SQL migrations embedded in the binary.
package migrate

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/bitofbytes-io/dined/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

// Apply runs any pending migrations against pool before the app serves
// traffic. It records versions in goose's default goose_db_version table, the
// same table the goose CLI (make migrate) uses, so migrations already applied
// by either path are never re-run.
func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	return apply(ctx, pool, migrations.Files)
}

func apply(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS) error {
	// A session-level advisory lock serializes concurrent startups (e.g. two
	// replicas during a rolling update): the second waits, then finds nothing
	// left to apply. Poll every second for up to five minutes.
	locker, err := lock.NewPostgresSessionLocker(lock.WithLockTimeout(1, 300))
	if err != nil {
		return fmt.Errorf("migrate: create session locker: %w", err)
	}

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, fsys, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("migrate: create goose provider: %w", err)
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("migrate: goose up: %w", err)
	}
	for _, result := range results {
		slog.Info("applied database migration", "file", result.Source.Path, "version", result.Source.Version, "duration", result.Duration)
	}

	version, err := provider.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("migrate: read goose version: %w", err)
	}
	slog.Info("database migrations up to date", "version", version, "applied", len(results))
	return nil
}
