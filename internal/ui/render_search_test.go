package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

func TestRenderSearchShowsRemoveOnlyForZeroVisitSavedSpots(t *testing.T) {
	orphanID := uuid.New()
	visitedID := uuid.New()
	var out strings.Builder
	err := Render(&out, "search", PageData{
		Authenticated: true,
		Query:         "Amigos",
		SearchResults: []RestaurantResult{
			{
				Restaurant: model.Restaurant{ID: orphanID, Name: "Amigos"},
				VisitCount: 0,
			},
			{
				Restaurant: model.Restaurant{ID: visitedID, Name: "Tupelo Honey"},
				LatestVisit: &model.Visit{
					VisitedAt:  time.Date(2026, 5, 16, 18, 30, 0, 0, time.UTC),
					Picker:     &model.Person{Name: "Daniel"},
					PriceLevel: 2,
				},
				VisitCount:    1,
				AverageRating: 7.9,
				HasAverage:    true,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "<span>Avg 7.9</span>") || !strings.Contains(rendered, "<span>Avg -</span>") {
		t.Fatalf("rendered search should show an average only where one counts:\n%s", rendered)
	}
	if got := strings.Count(rendered, "history-remove-button"); got != 1 {
		t.Fatalf("rendered %d remove buttons, want 1:\n%s", got, rendered)
	}
	if !strings.Contains(rendered, `action="/restaurants/`+orphanID.String()+`/delete"`) {
		t.Fatalf("rendered search missing orphan delete form:\n%s", rendered)
	}
	for _, fragment := range []string{
		`hx-boost="false"`,
		`data-delete-title="Remove saved spot?"`,
		`data-delete-confirm="Remove"`,
		`data-delete-form`,
		`<dialog class="confirm-modal" id="delete-dine-modal"`,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered search missing %q:\n%s", fragment, rendered)
		}
	}
	if !strings.Contains(rendered, `name="q" value="Amigos"`) {
		t.Fatalf("rendered search missing preserved query input:\n%s", rendered)
	}
	if strings.Contains(rendered, `onsubmit="return confirm`) {
		t.Fatalf("rendered search still uses inline browser confirm:\n%s", rendered)
	}
	if strings.Contains(rendered, `action="/restaurants/`+visitedID.String()+`/delete"`) {
		t.Fatalf("rendered search allowed removing visited restaurant:\n%s", rendered)
	}
	if strings.Contains(rendered, "Google Places results will appear here") {
		t.Fatalf("rendered search showed Google Places warning with saved spots:\n%s", rendered)
	}
}

func TestRenderSearchDoesNotShowGooglePlacesWarningForEmptySavedSearch(t *testing.T) {
	var out strings.Builder
	err := Render(&out, "search", PageData{Query: "Amigos"})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "No saved dines match that search yet.") {
		t.Fatalf("rendered search missing saved-search empty state:\n%s", rendered)
	}
	if strings.Contains(rendered, "Google Places results will appear here") {
		t.Fatalf("rendered search showed Google Places warning:\n%s", rendered)
	}
}
