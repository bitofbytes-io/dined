package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAssetAppendsStaticFileVersion(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "styles.css"), []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	modTime := time.Unix(1715451234, 0)
	if err := os.Chtimes(filepath.Join(dir, "styles.css"), modTime, modTime); err != nil {
		t.Fatal(err)
	}
	loadTestAssetVersions(t, dir)

	got := string(asset("/static/styles.css"))
	want := "/static/styles.css?v=1715451234"
	if got != want {
		t.Fatalf("asset() = %q, want %q", got, want)
	}
}

func TestAssetFallsBackWhenFileMissing(t *testing.T) {
	loadTestAssetVersions(t, t.TempDir())

	got := string(asset("/static/missing.css"))
	want := "/static/missing.css"
	if got != want {
		t.Fatalf("asset() = %q, want %q", got, want)
	}
}

func TestAssetDoesNotVersionPathsOutsideStatic(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "static")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example"), 0o644); err != nil {
		t.Fatal(err)
	}
	loadTestAssetVersions(t, dir)

	got := string(asset("/static/../go.mod"))
	if got != "/static/../go.mod" {
		t.Fatalf("asset() = %q, want original path", got)
	}
}

func TestRenderUsesVersionedStylesheetAndScript(t *testing.T) {
	dir := t.TempDir()
	files := []string{"styles.css", "htmx.min.js"}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(file), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	loadTestAssetVersions(t, dir)

	var out strings.Builder
	if err := Render(&out, "login", PageData{}); err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{`href="/static/styles.css?v=`, `src="/static/htmx.min.js?v=`} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered HTML missing %q:\n%s", fragment, rendered)
		}
	}
}

func TestRenderLoginOAuthLinkOptsOutOfBoost(t *testing.T) {
	var out strings.Builder
	if err := Render(&out, "login", PageData{}); err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, `href="/api/auth/google" hx-boost="false"`) {
		t.Fatalf("rendered login missing unboosted OAuth link:\n%s", rendered)
	}
}

func TestAppScriptIsStaticAndHandlesPageBehaviour(t *testing.T) {
	script := appScript(t)
	if strings.Contains(script, "{{") {
		t.Fatal("app.js contains template actions, but it is served as a static file")
	}
	for _, fragment := range []string{
		`function dinedUpdateLogValidation(form)`,
		`function dinedCompressPhoto(file, maxBytes)`,
		`uploader.dataset.photoMaxBytes`,
		`function dinedConfirmDelete(event, form)`,
		`document.addEventListener("htmx:load", dinedInitializePage);`,
	} {
		if !strings.Contains(script, fragment) {
			t.Fatalf("app.js missing %q", fragment)
		}
	}
}
