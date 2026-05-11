package config

import (
	"os"
	"path/filepath"
	"testing"
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
