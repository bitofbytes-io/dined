package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGetEnvOrFilePrefersEnv(t *testing.T) {
	t.Setenv("DINED_TEST_SECRET", "env-value")
	got, err := getEnvOrFile("DINED_TEST_SECRET", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "env-value" {
		t.Fatalf("got %q", got)
	}
}

func TestGetEnvOrFileReadsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte(" file-value \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DINED_TEST_SECRET_FILE", path)
	got, err := getEnvOrFile("DINED_TEST_SECRET", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "file-value" {
		t.Fatalf("got %q", got)
	}
}

func TestLoadOAuthConfig(t *testing.T) {
	t.Setenv("PORT", "4601")
	t.Setenv("DATA_STORE", "memory")
	t.Setenv("AUTH_GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("AUTH_GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("AUTH_GOOGLE_REDIRECT_URL", "http://localhost:4600/api/auth/google/callback")
	t.Setenv("AUTH_GOOGLE_ALLOWED_EMAILS", "one@example.com, two@example.com")
	t.Setenv("AUTH_GOOGLE_ALLOWED_DOMAINS", "example.org")
	t.Setenv("AUTH_SESSION_TTL", "24h")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GoogleClientID != "client-id" {
		t.Fatalf("got client id %q", cfg.GoogleClientID)
	}
	if cfg.GoogleRedirectURL != "http://localhost:4600/api/auth/google/callback" {
		t.Fatalf("got redirect url %q", cfg.GoogleRedirectURL)
	}
	if len(cfg.GoogleAllowedEmails) != 2 {
		t.Fatalf("got allowed emails %#v", cfg.GoogleAllowedEmails)
	}
	if len(cfg.GoogleAllowedDomains) != 1 {
		t.Fatalf("got allowed domains %#v", cfg.GoogleAllowedDomains)
	}
	if cfg.AuthSessionTTL != 24*time.Hour {
		t.Fatalf("got ttl %s", cfg.AuthSessionTTL)
	}
}

func TestLoadRequiresOAuthAllowlist(t *testing.T) {
	t.Setenv("DATA_STORE", "memory")
	t.Setenv("AUTH_GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("AUTH_GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("AUTH_GOOGLE_ALLOWED_EMAILS", "")
	t.Setenv("AUTH_GOOGLE_ALLOWED_DOMAINS", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected error")
	}
}
