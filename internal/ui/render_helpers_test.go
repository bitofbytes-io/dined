package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bitofbytes-io/dined/internal/testutil"
)

var testPhotoDataURI = testutil.PhotoURI(1)

func loadTestAssetVersions(t *testing.T, dir string) {
	t.Helper()
	if err := LoadAssetVersions(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { assetVersions.Store(nil) })
}

// staticScripts returns the entry point static/app.js and the feature modules
// in static/js, keyed by path relative to static.
func staticScripts(t *testing.T) map[string]string {
	t.Helper()
	root := filepath.Join("..", "..", "static")
	scripts := map[string]string{}
	for _, pattern := range []string{"app.js", filepath.Join("js", "*.js")} {
		paths, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				t.Fatal(err)
			}
			scripts[filepath.ToSlash(rel)] = string(data)
		}
	}
	if len(scripts) < 2 {
		t.Fatalf("found %d static scripts, want app.js and its feature modules", len(scripts))
	}
	return scripts
}

// appScript returns every static script joined, for checks on behaviour that
// may live in any feature module.
func appScript(t *testing.T) string {
	t.Helper()
	var all strings.Builder
	for _, script := range staticScripts(t) {
		all.WriteString(script)
		all.WriteString("\n")
	}
	return all.String()
}
