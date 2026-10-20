// photo-storage audits photos by default. -apply explicitly enables a bounded,
// resumable binary backfill after migration 008. It never runs schema migrations.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	apply := flag.Bool("apply", false, "backfill missing binary photo representations (writes)")
	batch := flag.Int("batch", 25, "backfill batch size (1..100)")
	limit := flag.Int("max-rows", 1000, "maximum rows per audit/backfill run (1..10000)")
	timeout := flag.Duration("timeout", 5*time.Minute, "maximum run duration")
	flag.Parse()
	if err := run(*apply, *batch, *limit, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, "photo storage:", safeDatabaseError(err))
		os.Exit(1)
	}
}

func run(apply bool, batch, limit int, timeout time.Duration) error {
	cfg, err := databasePoolConfig()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("cannot initialize database pool")
	}
	defer pool.Close()
	store := repository.New(pool)
	if apply {
		updated, remaining, err := store.BackfillPhotos(ctx, batch, limit)
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "backfill updated=%d remaining=%d\n", updated, remaining)
		if remaining != 0 {
			return fmt.Errorf("backfill incomplete; rerun this bounded command, and rerun after all old replicas stop")
		}
	}
	enc := json.NewEncoder(os.Stdout)
	missing := 0
	count, err := store.AuditPhotos(ctx, limit, func(p repository.PhotoAudit) error {
		if !p.BinaryPresent || !p.ThumbnailPresent {
			missing++
		}
		return enc.Encode(p)
	})
	if err != nil {
		return err
	}
	if apply && missing != 0 {
		return fmt.Errorf("audit found %d legacy-only rows after backfill; old writers may still be active, rerun after convergence", missing)
	}
	fmt.Fprintf(os.Stderr, "complete audit: photos=%d\n", count)
	return nil
}

func databasePoolConfig() (*pgxpool.Config, error) {
	dsn, err := config.LoadDatabaseURL()
	if err != nil {
		return nil, fmt.Errorf("cannot read database configuration")
	}
	if dsn == "" {
		return nil, fmt.Errorf("database configuration is required")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid database configuration")
	}
	cfg.MaxConns = 1
	return cfg, nil
}

// Driver errors may include connection details or database row values. Keep
// operator diagnostics useful without echoing those values to stdout/stderr.
func safeDatabaseError(err error) error {
	var connection *pgconn.ConnectError
	if errors.As(err, &connection) {
		return fmt.Errorf("database connection failed")
	}
	var database *pgconn.PgError
	if errors.As(err, &database) {
		return fmt.Errorf("database operation failed (SQLSTATE %s)", database.Code)
	}
	return err
}
