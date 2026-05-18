package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/bitofbytes-io/dined/internal/auth"
	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/bitofbytes-io/dined/internal/server"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	level := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	store, authRepo, cleanup := openStores(cfg)
	defer cleanup()

	authService := auth.NewService(authRepo, cfg.AuthSessionTTL)
	googleAuth, err := auth.NewGoogleAuthenticator(
		context.Background(),
		cfg.GoogleClientID,
		cfg.GoogleClientSecret,
		cfg.GoogleRedirectURL,
		cfg.GoogleAllowedDomains,
		cfg.GoogleAllowedEmails,
	)
	if err != nil {
		slog.Error("initialize google auth", "error", err)
		os.Exit(1)
	}

	placesClient := places.NewClient(cfg.GooglePlacesAPIKey)
	srv := server.New(cfg, store, placesClient, authService, googleAuth)

	httpServer := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Router(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("starting dined", "port", cfg.Port)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func openStores(cfg *config.Config) (repository.DinerStore, auth.Repository, func()) {
	if cfg.DataStore == config.DataStoreMemory {
		slog.Info("using in-memory data store")
		return repository.NewMemoryStore(), auth.NewMemoryRepository(), func() {}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Error("connect database", "error", err)
		os.Exit(1)
	}

	if err := pool.Ping(ctx); err != nil {
		slog.Error("ping database", "error", err)
		pool.Close()
		os.Exit(1)
	}

	return repository.New(pool), auth.NewPostgresRepository(pool), pool.Close
}
