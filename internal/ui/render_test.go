package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/apptime"
)

func TestAssetAppendsStaticFileVersion(t *testing.T) {
	withWorkingDir(t)
	if err := os.Mkdir("static", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("static", "styles.css"), []byte("body{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	modTime := time.Unix(1715451234, 0)
	if err := os.Chtimes(filepath.Join("static", "styles.css"), modTime, modTime); err != nil {
		t.Fatal(err)
	}

	got := string(asset("/static/styles.css"))
	want := "/static/styles.css?v=1715451234"
	if got != want {
		t.Fatalf("asset() = %q, want %q", got, want)
	}
}

func TestAssetFallsBackWhenFileMissing(t *testing.T) {
	withWorkingDir(t)

	got := string(asset("/static/missing.css"))
	want := "/static/missing.css"
	if got != want {
		t.Fatalf("asset() = %q, want %q", got, want)
	}
}

func TestAssetDoesNotVersionPathsOutsideStatic(t *testing.T) {
	withWorkingDir(t)
	if err := os.Mkdir("static", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("go.mod", []byte("module example"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := string(asset("/static/../go.mod"))
	if got != "/static/../go.mod" {
		t.Fatalf("asset() = %q, want original path", got)
	}
	if strings.Contains(got, "?v=") {
		t.Fatalf("asset() versioned a path outside static: %q", got)
	}
}

func TestRenderUsesVersionedStylesheetAndScript(t *testing.T) {
	withWorkingDir(t)
	if err := os.Mkdir("static", 0o755); err != nil {
		t.Fatal(err)
	}
	files := []string{"styles.css", "htmx.min.js"}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join("static", file), []byte(file), 0o644); err != nil {
			t.Fatal(err)
		}
	}

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

func TestRenderDefaultsNowLocalToEasternTime(t *testing.T) {
	before := apptime.FormatDatetimeLocal(time.Now())
	var out strings.Builder
	if err := Render(&out, "log", PageData{}); err != nil {
		t.Fatal(err)
	}
	after := apptime.FormatDatetimeLocal(time.Now())

	rendered := out.String()
	if !strings.Contains(rendered, `value="`+before+`"`) && !strings.Contains(rendered, `value="`+after+`"`) {
		t.Fatalf("rendered HTML did not include Eastern datetime-local default %q or %q:\n%s", before, after, rendered)
	}
}

func withWorkingDir(t *testing.T) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatal(err)
		}
	})
}
