package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

func TestRenderTrophyShowsRedesignedRecordsAndTopRestaurants(t *testing.T) {
	var out strings.Builder
	data := PageData{
		Stats: model.Stats{
			TotalDines:         12,
			AverageRating:      8.4,
			BestPicker:         "Jen",
			BestPickerAverage:  8.8,
			WorstPicker:        "Daniel",
			WorstPickerAverage: 6.9,
			NewPlaces:          7,
			CitiesExplored:     3,
			TopRestaurants: []model.RestaurantRatingStat{
				{Name: "El Patio Verde", AverageRating: 8.6, RatingCount: 4, VisitCount: 1},
			},
			TopRestaurantsByCuisine: []model.CuisineRestaurantStat{
				{Cuisine: "Mexican", Name: "El Patio Verde", AverageRating: 8.6, RatingCount: 4, VisitCount: 1},
				{Cuisine: "Southern", Name: "Tupelo Honey", AverageRating: 8.2, RatingCount: 4, VisitCount: 1},
			},
			BiggestSplitRestaurant: "Saffron Counter",
			BiggestSplit:           4.5,
			MostLoyalRestaurant:    "Hank's Downtown Diner",
			MostLoyalVisits:        6,
			NostalgiaRestaurant:    "Bida Manda",
			NostalgiaLastVisitedAt: time.Date(2025, 3, 14, 23, 0, 0, 0, time.UTC),
			TasteProfiles: []model.PersonTasteStat{
				{Person: "Caleb", Category: "Indian", AverageRating: 9.5, RatingCount: 2},
			},
		},
	}
	if err := Render(&out, "trophy", data); err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{
		"Total Dines",
		"Family Average",
		"Best Picker",
		"Worst Picker",
		"Daniel",
		"6.9 average",
		"New Places",
		"Cities Explored",
		"All-Time Top Restaurants",
		"Best By Cuisine",
		"Mexican",
		"El Patio Verde",
		"8.6 avg",
		"Southern",
		"Tupelo Honey",
		"8.2 avg",
		"Biggest Split",
		"Saffron Counter",
		"4.5 point gap",
		"Most Loyal",
		"6 visits",
		"Haven't Been Since",
		"Bida Manda",
		"Mar 14, 2025",
		"Taste Profiles",
		"Caleb",
		"Indian",
		"9.5 avg",
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered trophy missing %q:\n%s", fragment, rendered)
		}
	}
	if got := strings.Count(rendered, `class="record"`); got != 9 {
		t.Fatalf("rendered %d trophy records, want 9:\n%s", got, rendered)
	}
	for _, oldAward := range []string{"Safe Bet", "The Regular", "Table Divided", "Next Up"} {
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
	if !strings.Contains(rendered, "All-Time Top Restaurants") || !strings.Contains(rendered, "Waiting on more dines") || !strings.Contains(rendered, "Waiting on cuisine data") ||
		!strings.Contains(rendered, "Waiting on cuisine ratings") || !strings.Contains(rendered, "No repeats yet") {
		t.Fatalf("rendered trophy missing top restaurant empty state:\n%s", rendered)
	}
	if !strings.Contains(rendered, "No mapped dines yet") {
		t.Fatalf("rendered trophy missing map empty state:\n%s", rendered)
	}
}

func TestRenderTrophyShowsMapProxyImageWithoutGoogleKey(t *testing.T) {
	var out strings.Builder
	err := Render(&out, "trophy", PageData{
		TrophyMapReady: true,
		TrophyMapPoints: []model.RestaurantMapPoint{
			{RestaurantID: uuid.New(), Name: "Hank's Downtown Diner", Latitude: 35.7796, Longitude: -78.6382, VisitCount: 1, LatestVisitedAt: time.Now()},
		},
		TrophyMapLabels: []TrophyMapLabel{
			{Name: "Hank's Downto...", Left: "50.000%", Top: "50.000%"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{
		"Places We've Dined",
		"1 pinned",
		`class="dined-map-preview"`,
		`aria-controls="trophy-map-modal"`,
		`dinedOpenTrophyMap()`,
		`id="trophy-map-modal"`,
		`class="trophy-map-stage"`,
		`class="trophy-map-label"`,
		"Hank&#39;s Downto...",
		`src="/trophy-case/map.png"`,
		`id="photo-preview-modal"`,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered trophy missing %q:\n%s", fragment, rendered)
		}
	}
	for _, fragment := range []string{"maps.googleapis.com", "key=", "GOOGLE_PLACES_API_KEY"} {
		if strings.Contains(rendered, fragment) {
			t.Fatalf("rendered trophy leaked %q:\n%s", fragment, rendered)
		}
	}
}
