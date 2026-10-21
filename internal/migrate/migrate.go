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
	return applyWithTimeouts(ctx, pool, fsys, startupTimeouts)
}

// applyWithTimeouts shares one startup deadline across all Goose work while
// allowing regression tests to exercise expiry with short, isolated budgets.
func applyWithTimeouts(ctx context.Context, pool *pgxpool.Pool, fsys fs.FS, timeouts migrationTimeouts) (retErr error) {
	// One deadline includes connection acquisition, lock waiting, SQL, and the
	// final version check. Cleanup has a separate, bounded allowance.
	ctx, cancel := context.WithTimeout(ctx, timeouts.total)
	defer cancel()
	defer func() {
		if err := ctx.Err(); err != nil {
			// Do not log the SQL or connection details from a canceled operation.
			retErr = fmt.Errorf("migrate: startup interrupted: %w", err)
		}
	}()
	locker, err := lock.NewPostgresSessionLocker(
		lock.WithLockTimeout(1, retryCount(timeouts.lock)),
		lock.WithUnlockTimeout(1, retryCount(timeouts.unlock)),
	)
	if err != nil {
		return fmt.Errorf("migrate: create session locker: %w", err)
	}
	boundedLocker := deadlineLocker{locker: locker, timeouts: timeouts}

	db := stdlib.OpenDBFromPool(pool)
	defer db.Close()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, fsys, goose.WithSessionLocker(boundedLocker))
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

	// GetVersions reads without acquiring the session lock again. Up already
	// serialized pending work; logging must not restart the lock-wait budget.
	version, _, err := provider.GetVersions(ctx)
	if err != nil {
		return fmt.Errorf("migrate: read goose version: %w", err)
	}
	slog.Info("database migrations up to date", "version", version, "applied", len(results))
	return nil
}
