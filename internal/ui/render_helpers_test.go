package ui

import (
	"os"
	"path/filepath"
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

func appScript(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "static", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
