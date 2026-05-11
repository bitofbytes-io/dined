package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Port               string
	DataStore          string
	DatabaseURL        string
	APIToken           string
	GooglePlacesAPIKey string
	LogLevel           string
	SecureCookies      bool
}

const (
	DataStoreMemory   = "memory"
	DataStorePostgres = "postgres"
)

func Load() (*Config, error) {
	cfg := &Config{}
	var err error

	if cfg.Port, err = getEnv("PORT", "4600"); err != nil {
		return nil, err
	}
	if cfg.DataStore, err = getEnv("DATA_STORE", DataStorePostgres); err != nil {
		return nil, err
	}
	if cfg.DatabaseURL, err = getEnvOrFile("DATABASE_URL", "/run/secrets/dined_database_url"); err != nil {
		return nil, err
	}
	if cfg.APIToken, err = getEnvOrFile("API_TOKEN", "/run/secrets/dined_api_token"); err != nil {
		return nil, err
	}
	if cfg.GooglePlacesAPIKey, err = getEnvOrFile("GOOGLE_PLACES_API_KEY", "/run/secrets/dined_google_places_api_key"); err != nil {
		return nil, err
	}
	if cfg.LogLevel, err = getEnv("LOG_LEVEL", "info"); err != nil {
		return nil, err
	}
	secure, err := getEnv("SECURE_COOKIES", "true")
	if err != nil {
		return nil, err
	}
	cfg.SecureCookies = secure != "false"

	if cfg.DataStore != DataStoreMemory && cfg.DataStore != DataStorePostgres {
		return nil, fmt.Errorf("DATA_STORE must be memory or postgres, got %q", cfg.DataStore)
	}
	if cfg.DataStore == DataStorePostgres && cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.APIToken == "" {
		return nil, fmt.Errorf("API_TOKEN is required")
	}
	if cfg.DataStore == DataStorePostgres && cfg.GooglePlacesAPIKey == "" {
		return nil, fmt.Errorf("GOOGLE_PLACES_API_KEY is required")
	}
	return cfg, nil
}

func getEnv(key, defaultVal string) (string, error) {
	if filePath := os.Getenv(key + "_FILE"); filePath != "" {
		return readSecret(filePath, key+"_FILE")
	}
	if val := os.Getenv(key); val != "" {
		return val, nil
	}
	return defaultVal, nil
}

func getEnvOrFile(key, defaultPath string) (string, error) {
	if val := os.Getenv(key); val != "" {
		return val, nil
	}
	if filePath := os.Getenv(key + "_FILE"); filePath != "" {
		return readSecret(filePath, key+"_FILE")
	}
	return readSecret(defaultPath, key)
}

func readSecret(path, name string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("config: reading %s (%s): %w", name, path, err)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return "", fmt.Errorf("config: %s (%s) is empty", name, path)
	}
	return value, nil
}
