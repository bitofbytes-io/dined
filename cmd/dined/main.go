package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bitofbytes-io/dined/internal/auth"
	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/metrics"
	"github.com/bitofbytes-io/dined/internal/migrate"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/bitofbytes-io/dined/internal/server"
	"github.com/bitofbytes-io/dined/internal/ui"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	serverReadHeaderTimeout = 5 * time.Second
	// serverRequestTimeout bounds reading a request body (a dine form carries
	// up to four ~500 KB photos as base64) and writing the response, which may
	// wait on Google Places, over a slow mobile connection.
	serverRequestTimeout = 2 * time.Minute
	serverIdleTimeout    = 2 * time.Minute
	shutdownTimeout      = 15 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("dined stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel(cfg.LogLevel)})))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, authRepo, pool, closeStores, err := openStores(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeStores()

	var dbCollectors []prometheus.Collector
	if pool != nil {
		dbCollectors = append(dbCollectors, metrics.NewPoolCollector(pool))
	}

	allowlist := auth.NewAllowlist(cfg.GoogleAllowedEmails, cfg.GoogleAllowedDomains)
	authService := auth.NewService(authRepo, cfg.AuthSessionTTL, allowlist.Allowed)
	googleAuth, err := auth.NewGoogleAuthenticator(
		context.Background(),
		cfg.GoogleClientID,
		cfg.GoogleClientSecret,
		cfg.GoogleRedirectURL,
		allowlist,
	)
	if err != nil {
		return err
	}

	if err := ui.LoadAssetVersions("static"); err != nil {
		slog.Warn("static asset versions unavailable; serving unversioned URLs", "error", err)
	}

	placesClient := places.NewClient(cfg.GooglePlacesAPIKey)
	srv := server.New(cfg, store, placesClient, authService, googleAuth)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Router(),
		ReadHeaderTimeout: serverReadHeaderTimeout,
		ReadTimeout:       serverRequestTimeout,
		WriteTimeout:      serverRequestTimeout,
		IdleTimeout:       serverIdleTimeout,
	}

	schedulerCtx, stopScheduler := context.WithCancel(context.Background())
	var scheduler sync.WaitGroup
	scheduler.Add(1)
	go func() {
		defer scheduler.Done()
		authService.ScheduleSessionCleanup(schedulerCtx, auth.SessionCleanupInterval)
	}()
	// Stop the cleanup before the deferred store close runs.
	defer func() {
		stopScheduler()
		scheduler.Wait()
	}()

	// Bind the public port before the metrics listener, so a METRICS_ADDR that
	// collides with PORT costs metrics rather than the app.
	publicListener, err := net.Listen("tcp", httpServer.Addr)
	if err != nil {
		return err
	}

	// The metrics listener is internal-only and optional: it is never mounted on
	// the public router, and a bind failure is logged without stopping dined.
	metricsServer := metrics.Start(cfg.MetricsAddr, metrics.NewRegistry(dbCollectors...))
	// Runs before the scheduler stops and the store closes, after the public
	// server has shut down.
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			slog.Warn("metrics listener shutdown", "error", err)
		}
	}()

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("starting dined", "port", cfg.Port)
		serveErr <- httpServer.Serve(publicListener)
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down dined")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}

func slogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// openStores also returns the Postgres pool for metrics; it is nil for the
// memory store.
func openStores(ctx context.Context, cfg *config.Config) (repository.DinerStore, auth.Repository, *pgxpool.Pool, func(), error) {
	if cfg.DataStore == config.DataStoreMemory {
		slog.Info("using in-memory data store")
		return repository.NewMemoryStore(), auth.NewMemoryRepository(), nil, func() {}, nil
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(connectCtx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("connect database: %w", err)
	}

	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, nil, nil, nil, fmt.Errorf("ping database: %w", err)
	}

	// Apply pending migrations before serving. A failure exits the process, so
	// a deploy whose migration fails never becomes healthy and stops rolling out.
	if err := migrate.Apply(ctx, pool); err != nil {
		pool.Close()
		return nil, nil, nil, nil, err
	}

	return repository.New(pool), auth.NewPostgresRepository(pool), pool, pool.Close, nil
}
