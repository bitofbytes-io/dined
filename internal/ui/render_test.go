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

func TestRenderTrophyShowsRedesignedRecordsAndTopRestaurants(t *testing.T) {
	var out strings.Builder
	data := PageData{
		Stats: model.Stats{
			TotalDines:        12,
			AverageRating:     8.4,
			BestPicker:        "Jen",
			BestPickerAverage: 8.8,
			NewPlaces:         7,
			CitiesExplored:    3,
			TopRestaurants: []model.RestaurantRatingStat{
				{Name: "El Patio Verde", AverageRating: 8.6, RatingCount: 4, VisitCount: 1},
			},
		},
		PickerTurn: model.PickerTurn{NextPicker: model.Person{Name: "Caleb"}},
	}
	if err := Render(&out, "trophy", data); err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{
		"Total Dines",
		"Family Average",
		"Best Picker",
		"Next Up",
		"New Places",
		"Cities Explored",
		"All-Time Top Restaurants",
		"El Patio Verde",
		"8.6 avg",
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered trophy missing %q:\n%s", fragment, rendered)
		}
	}
	if got := strings.Count(rendered, `class="record"`); got != 6 {
		t.Fatalf("rendered %d trophy records, want 6:\n%s", got, rendered)
	}
	for _, oldAward := range []string{"Safe Bet", "The Regular", "Table Divided"} {
		if strings.Contains(rendered, oldAward) {
			t.Fatalf("rendered retired duplicate award %q:\n%s", oldAward, rendered)
		}
	}
}

func TestRenderTrophyTopRestaurantsEmptyState(t *testing.T) {
	var out strings.Builder
	if err := Render(&out, "trophy", PageData{}); err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "All-Time Top Restaurants") || !strings.Contains(rendered, "Waiting on more dines") {
		t.Fatalf("rendered trophy missing top restaurant empty state:\n%s", rendered)
	}
}

func TestRenderRestaurantShowsGoogleRefreshAction(t *testing.T) {
	restaurantID := uuid.New()
	placeID := "place-1"
	var out strings.Builder
	err := Render(&out, "restaurant", PageData{
		Authenticated: true,
		Notice:        "Google info refreshed",
		Restaurant: &model.Restaurant{
			ID:            restaurantID,
			Name:          "Tupelo Honey Southern Kitchen & Bar",
			GooglePlaceID: &placeID,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{
		"Google info refreshed",
		`action="/restaurants/` + restaurantID.String() + `/google-refresh"`,
		"Refresh Google Info",
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered restaurant missing %q:\n%s", fragment, rendered)
		}
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

func TestRenderLogPreservesPrefillCity(t *testing.T) {
	var out strings.Builder
	err := Render(&out, "log", PageData{PrefillCity: "Apex"})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `name="city" value="Apex"`) {
		t.Fatalf("rendered log missing city hidden input:\n%s", got)
	}
	if strings.Contains(got, `if (city) city.value = "";`) {
		t.Fatalf("rendered log clears hidden city on unmatched datalist input:\n%s", got)
	}
	if !strings.Contains(got, `if (city) city.value = option.dataset.city || "";`) {
		t.Fatalf("rendered log does not overwrite city on matched datalist input:\n%s", got)
	}
}

func TestRenderLogCarriesGoogleMetadataPrefill(t *testing.T) {
	var out strings.Builder
	err := Render(&out, "log", PageData{
		PrefillPhone:            "919-555-0100",
		PrefillWebsite:          "https://example.com",
		PrefillGoogleRating:     "4.7",
		PrefillGooglePriceLevel: "3",
		PrefillLatitude:         "35.779600",
		PrefillLongitude:        "-78.638200",
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{
		`name="phone" value="919-555-0100"`,
		`name="website" value="https://example.com"`,
		`name="google_rating" value="4.7"`,
		`name="google_price_level" value="3"`,
		`name="latitude" value="35.779600"`,
		`name="longitude" value="-78.638200"`,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered log missing %q:\n%s", fragment, rendered)
		}
	}
}

func TestRenderAuthenticatedDinesUsesEditLinkAndDeleteConfirmationModal(t *testing.T) {
	visitID := uuid.New()
	restaurantID := uuid.New()
	var out strings.Builder
	err := Render(&out, "dines", PageData{
		Authenticated: true,
		Visits: []model.Visit{{
			ID:         visitID,
			Restaurant: model.Restaurant{ID: restaurantID, Name: "Hank's"},
			VisitedAt:  time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
			Picker:     model.Person{Name: "Daniel"},
			PriceLevel: 2,
			Ratings:    []model.Rating{{Person: model.Person{Name: "Daniel"}, Score: 8}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	rendered := out.String()
	for _, fragment := range []string{
		`/visits/` + visitID.String() + `/edit`,
		`action="/visits/` + visitID.String() + `/delete" hx-boost="false" data-delete-dine-form`,
		`onsubmit="return dinedConfirmDelete(event, this)"`,
		`<dialog class="confirm-modal" id="delete-dine-modal"`,
		`id="delete-dine-confirm-form"`,
		`Delete this dine?`,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered dines missing %q:\n%s", fragment, rendered)
		}
	}
	if strings.Contains(rendered, `onsubmit="return confirm`) {
		t.Fatalf("rendered dines still uses inline browser confirm:\n%s", rendered)
	}
}

func TestRenderVisitEditPrefillsRatingsTagsAndNotes(t *testing.T) {
	danielID := uuid.New()
	jenID := uuid.New()
	tagID := uuid.New()
	visitID := uuid.New()
	restaurantID := uuid.New()
	visitedAt := time.Date(2026, 5, 16, 19, 30, 0, 0, apptime.EasternLocation())
	note := "Updated score after dessert."
	var out strings.Builder
	err := Render(&out, "visit-edit", PageData{
		Visit: &model.Visit{
			ID:         visitID,
			Restaurant: model.Restaurant{ID: restaurantID, Name: "Hank's"},
			VisitedAt:  visitedAt,
			Picker:     model.Person{ID: jenID, Name: "Jen"},
			PriceLevel: 3,
			Notes:      &note,
			Ratings:    []model.Rating{{Person: model.Person{ID: danielID, Name: "Daniel"}, Score: 8.5}},
			Tags:       []model.Tag{{ID: tagID, Name: "Would Return"}},
		},
		People: []model.Person{{ID: danielID, Name: "Daniel"}, {ID: jenID, Name: "Jen"}},
		Tags:   []model.Tag{{ID: tagID, Name: "Would Return"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{
		`action="/visits/` + visitID.String() + `"`,
		`name="restaurant_id" value="` + restaurantID.String() + `"`,
		`value="` + apptime.FormatDatetimeLocal(visitedAt) + `"`,
		`value="` + jenID.String() + `" selected>Jen</option>`,
		`name="rating_` + danielID.String() + `" type="number" min="0" max="10" step="0.5" inputmode="decimal" placeholder="0-10" value="8.5"`,
		`value="` + tagID.String() + `" checked>`,
		note,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered visit edit missing %q:\n%s", fragment, rendered)
		}
	}
}

func TestRenderRestaurantEditPrefillsOverrideFields(t *testing.T) {
	restaurantID := uuid.New()
	address := "202 Corrected Way"
	city := "Garner"
	phone := "919-555-0199"
	website := "https://hanks.example"
	category := "Southern"
	rating := 4.9
	price := 2
	var out strings.Builder
	err := Render(&out, "restaurant-edit", PageData{
		Restaurant: &model.Restaurant{
			ID:               restaurantID,
			Name:             "Hank's Corrected Diner",
			Address:          &address,
			City:             &city,
			Phone:            &phone,
			Website:          &website,
			GoogleRating:     &rating,
			GooglePriceLevel: &price,
			Category:         &category,
			IsChain:          true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{
		`action="/restaurants/` + restaurantID.String() + `"`,
		`name="restaurant_name" value="Hank&#39;s Corrected Diner"`,
		`name="category" placeholder="American, Southern, Sushi..." value="Southern"`,
		`name="address" placeholder="Optional" value="202 Corrected Way"`,
		`name="phone" placeholder="Optional" value="919-555-0199"`,
		`name="website" type="url" placeholder="https://example.com" value="https://hanks.example"`,
		`name="google_rating" type="number" min="0" max="5" step="0.1" inputmode="decimal" placeholder="0-5" value="4.9"`,
		`value="2" selected>$$</option>`,
		`name="is_chain" value="true" checked>`,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered restaurant edit missing %q:\n%s", fragment, rendered)
		}
	}
}

func TestRenderSearchShowsRemoveOnlyForZeroVisitSavedSpots(t *testing.T) {
	orphanID := uuid.New()
	visitedID := uuid.New()
	var out strings.Builder
	err := Render(&out, "search", PageData{
		Query: "Amigos",
		SearchResults: []RestaurantResult{
			{
				Restaurant: model.Restaurant{ID: orphanID, Name: "Amigos"},
				VisitCount: 0,
			},
			{
				Restaurant: model.Restaurant{ID: visitedID, Name: "Tupelo Honey"},
				LatestVisit: &model.Visit{
					VisitedAt:  time.Date(2026, 5, 16, 18, 30, 0, 0, time.UTC),
					Picker:     model.Person{Name: "Daniel"},
					PriceLevel: 2,
				},
				VisitCount:    1,
				AverageRating: 7.9,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	if got := strings.Count(rendered, "history-remove-button"); got != 1 {
		t.Fatalf("rendered %d remove buttons, want 1:\n%s", got, rendered)
	}
	if !strings.Contains(rendered, `action="/restaurants/`+orphanID.String()+`/delete"`) {
		t.Fatalf("rendered search missing orphan delete form:\n%s", rendered)
	}
	if !strings.Contains(rendered, `name="q" value="Amigos"`) {
		t.Fatalf("rendered search missing preserved query input:\n%s", rendered)
	}
	if strings.Contains(rendered, `action="/restaurants/`+visitedID.String()+`/delete"`) {
		t.Fatalf("rendered search allowed removing visited restaurant:\n%s", rendered)
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
