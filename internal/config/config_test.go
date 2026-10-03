package config

import (
	"net/netip"
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

func TestLoadTrustedProxies(t *testing.T) {
	t.Setenv("DATA_STORE", "memory")
	t.Setenv("AUTH_GOOGLE_CLIENT_ID", "client-id")
	t.Setenv("AUTH_GOOGLE_CLIENT_SECRET", "client-secret")
	t.Setenv("AUTH_GOOGLE_ALLOWED_EMAILS", "one@example.com")

	t.Setenv("TRUSTED_PROXY_CIDRS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !containsAddr(cfg.TrustedProxies, "10.0.1.4") || !containsAddr(cfg.TrustedProxies, "127.0.0.1") ||
		containsAddr(cfg.TrustedProxies, "192.168.1.20") || containsAddr(cfg.TrustedProxies, "172.17.0.1") || containsAddr(cfg.TrustedProxies, "203.0.113.7") {
		t.Fatalf("default trusted proxies = %v", cfg.TrustedProxies)
	}

	t.Setenv("TRUSTED_PROXY_CIDRS", "10.0.9.0/24, 192.0.2.10")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !containsAddr(cfg.TrustedProxies, "10.0.9.200") || !containsAddr(cfg.TrustedProxies, "192.0.2.10") || containsAddr(cfg.TrustedProxies, "10.0.3.4") {
		t.Fatalf("configured trusted proxies = %v", cfg.TrustedProxies)
	}

	t.Setenv("TRUSTED_PROXY_CIDRS", "::ffff:10.0.9.0/120, ::ffff:192.0.2.10")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !containsAddr(cfg.TrustedProxies, "10.0.9.200") || !containsAddr(cfg.TrustedProxies, "192.0.2.10") || containsAddr(cfg.TrustedProxies, "10.0.8.1") {
		t.Fatalf("IPv4-mapped trusted proxies = %v", cfg.TrustedProxies)
	}

	t.Setenv("TRUSTED_PROXY_CIDRS", "::ffff:0:0/64")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for an IPv4-mapped prefix shorter than /96")
	}

	t.Setenv("TRUSTED_PROXY_CIDRS", "traefik")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for an invalid CIDR")
	}
}

func containsAddr(prefixes []netip.Prefix, addr string) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(netip.MustParseAddr(addr)) {
			return true
		}
	}
	return false
}
