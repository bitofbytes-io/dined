package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/bitofbytes-io/dined/internal/auth"
	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/bitofbytes-io/dined/internal/server"
	"github.com/bitofbytes-io/dined/internal/ui"
	"github.com/jackc/pgx/v5/pgxpool"
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

	store, authRepo, closeStores, err := openStores(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeStores()

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

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("starting dined", "port", cfg.Port)
		serveErr <- httpServer.ListenAndServe()
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

func openStores(ctx context.Context, cfg *config.Config) (repository.DinerStore, auth.Repository, func(), error) {
	if cfg.DataStore == config.DataStoreMemory {
		slog.Info("using in-memory data store")
		return repository.NewMemoryStore(), auth.NewMemoryRepository(), func() {}, nil
	}

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(connectCtx, cfg.DatabaseURL)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("connect database: %w", err)
	}

	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, nil, nil, fmt.Errorf("ping database: %w", err)
	}

	return repository.New(pool), auth.NewPostgresRepository(pool), pool.Close, nil
}
