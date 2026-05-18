package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Port                 string
	DataStore            string
	DatabaseURL          string
	GooglePlacesAPIKey   string
	LogLevel             string
	SecureCookies        bool
	GoogleClientID       string
	GoogleClientSecret   string
	GoogleRedirectURL    string
	GoogleAllowedDomains []string
	GoogleAllowedEmails  []string
	AuthSessionTTL       time.Duration
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
	if cfg.GooglePlacesAPIKey, err = getEnvOrFile("GOOGLE_PLACES_API_KEY", "/run/secrets/dined_google_places_api_key"); err != nil {
		return nil, err
	}
	if cfg.GoogleClientID, err = getEnvOrFile("AUTH_GOOGLE_CLIENT_ID", "/run/secrets/dined_google_client_id"); err != nil {
		return nil, err
	}
	if cfg.GoogleClientSecret, err = getEnvOrFile("AUTH_GOOGLE_CLIENT_SECRET", "/run/secrets/dined_google_client_secret"); err != nil {
		return nil, err
	}
	if cfg.GoogleRedirectURL, err = getEnv("AUTH_GOOGLE_REDIRECT_URL", "http://localhost:4600/api/auth/google/callback"); err != nil {
		return nil, err
	}
	cfg.GoogleAllowedDomains = parseCSV(os.Getenv("AUTH_GOOGLE_ALLOWED_DOMAINS"))
	cfg.GoogleAllowedEmails = parseCSV(os.Getenv("AUTH_GOOGLE_ALLOWED_EMAILS"))
	if cfg.LogLevel, err = getEnv("LOG_LEVEL", "info"); err != nil {
		return nil, err
	}
	secure, err := getEnv("SECURE_COOKIES", "true")
	if err != nil {
		return nil, err
	}
	cfg.SecureCookies = secure != "false"
	ttlValue, err := getEnv("AUTH_SESSION_TTL", "2160h")
	if err != nil {
		return nil, err
	}
	cfg.AuthSessionTTL, err = time.ParseDuration(ttlValue)
	if err != nil {
		return nil, fmt.Errorf("AUTH_SESSION_TTL must be a Go duration like 2160h, got %q", ttlValue)
	}

	if cfg.DataStore != DataStoreMemory && cfg.DataStore != DataStorePostgres {
		return nil, fmt.Errorf("DATA_STORE must be memory or postgres, got %q", cfg.DataStore)
	}
	if cfg.DataStore == DataStorePostgres && cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.DataStore == DataStorePostgres && cfg.GooglePlacesAPIKey == "" {
		return nil, fmt.Errorf("GOOGLE_PLACES_API_KEY is required")
	}
	if strings.TrimSpace(cfg.GoogleClientID) == "" {
		return nil, fmt.Errorf("AUTH_GOOGLE_CLIENT_ID is required")
	}
	if strings.TrimSpace(cfg.GoogleClientSecret) == "" {
		return nil, fmt.Errorf("AUTH_GOOGLE_CLIENT_SECRET is required")
	}
	if len(cfg.GoogleAllowedDomains) == 0 && len(cfg.GoogleAllowedEmails) == 0 {
		return nil, fmt.Errorf("AUTH_GOOGLE_ALLOWED_EMAILS or AUTH_GOOGLE_ALLOWED_DOMAINS is required")
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

func parseCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
