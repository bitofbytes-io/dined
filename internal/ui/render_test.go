package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
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
	if strings.Contains(rendered, "data-default-local-now") {
		t.Fatalf("rendered HTML still includes browser-local datetime override hook:\n%s", rendered)
	}
}

func TestRenderHomeShowsNextUp(t *testing.T) {
	var out strings.Builder
	if err := Render(&out, "home", PageData{PickerTurn: model.PickerTurn{NextPicker: model.Person{Name: "Jen"}}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Next Up: Jen") {
		t.Fatalf("rendered home missing next up copy:\n%s", out.String())
	}
}

func TestRenderTrophyShowsNextUpAward(t *testing.T) {
	var out strings.Builder
	if err := Render(&out, "trophy", PageData{PickerTurn: model.PickerTurn{NextPicker: model.Person{Name: "Jen"}}}); err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "<h2>Next Up</h2>") || !strings.Contains(rendered, "<p>Jen</p>") {
		t.Fatalf("rendered trophy missing next up award:\n%s", rendered)
	}
}

func TestRenderLogPreselectsNextPicker(t *testing.T) {
	jenID := uuid.New()
	var out strings.Builder
	err := Render(&out, "log", PageData{
		People: []model.Person{
			{ID: uuid.New(), Name: "Daniel"},
			{ID: jenID, Name: "Jen"},
		},
		PrefillPickerID: jenID.String(),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	want := `value="` + jenID.String() + `" selected>Jen</option>`
	if !strings.Contains(got, want) {
		t.Fatalf("rendered log missing selected picker %q:\n%s", want, got)
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
