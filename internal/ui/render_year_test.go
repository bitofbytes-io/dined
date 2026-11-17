package ui

import (
	"strings"
	"testing"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

func TestRenderYearShowsReview(t *testing.T) {
	tacoID, phoID := uuid.New(), uuid.New()
	var out strings.Builder
	data := PageData{
		YearPrev: 2024,
		YearNext: 2026,
		YearReview: &model.YearInReview{
			Year:              2025,
			TotalVisits:       6,
			MonthlyVisits:     [12]int{1, 0, 3, 0, 0, 0, 0, 0, 1, 0, 0, 1},
			NewRestaurants:    4,
			RepeatRestaurants: 1,
			MostVisited:       &model.YearRestaurantStat{ID: tacoID, Name: "Taco Town", VisitCount: 2},
			HighestRated:      &model.YearRestaurantStat{ID: phoID, Name: "Pho Place", AverageRating: 9.5},
			TopPicks:          []model.PersonTopPick{{Person: "Jen", RestaurantID: tacoID, Restaurant: "Taco Town", AverageRating: 8.5}},
		},
	}
	if err := Render(&out, "year", data); err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{
		"2025 in Review",
		"Dines By Month",
		"New Places",
		"Repeat Places",
		"Taco Town",
		"2 visits",
		`href="/restaurants/` + tacoID.String() + `"`,
		"Pho Place",
		"9.5 avg",
		"Top Pick By Person",
		"Jen",
		"8.5 avg",
		`href="/year/2024"`,
		`href="/year/2026"`,
		`href="/trophy-case"`,
		"--bar-height: 100%;",
		"--bar-height: 33%;",
		"--bar-height: 0%;",
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered year missing %q:\n%s", fragment, rendered)
		}
	}
	if got := strings.Count(rendered, `class="year-month-bar"`); got != 12 {
		t.Fatalf("rendered %d month bars, want 12", got)
	}
}

func TestRenderYearWithoutRepeatsOrRatings(t *testing.T) {
	var out strings.Builder
	data := PageData{YearReview: &model.YearInReview{Year: 2025, TotalVisits: 1, MonthlyVisits: [12]int{0, 1}, NewRestaurants: 1}}
	if err := Render(&out, "year", data); err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{"No repeats yet", "Waiting on ratings", "Waiting on rated picks"} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered year missing %q:\n%s", fragment, rendered)
		}
	}
	if strings.Contains(rendered, `href="/year/`) {
		t.Fatalf("rendered year links to neighboring years without any:\n%s", rendered)
	}
}

func TestRenderYearWithoutVisits(t *testing.T) {
	var out strings.Builder
	data := PageData{YearReview: &model.YearInReview{Year: 2019}, YearNext: 2020}
	if err := Render(&out, "year", data); err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "2019 in Review") || !strings.Contains(rendered, "No dines logged in 2019.") ||
		!strings.Contains(rendered, `href="/year/2020"`) {
		t.Fatalf("rendered empty year missing its empty state:\n%s", rendered)
	}
	if strings.Contains(rendered, "Dines By Month") {
		t.Fatalf("rendered a month chart for a year without dines:\n%s", rendered)
	}
}

func TestRenderTrophyLinksReviewYears(t *testing.T) {
	var out strings.Builder
	if err := Render(&out, "trophy", PageData{ReviewYears: []int{2026, 2025}}); err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{"Year In Review", `href="/year/2026"`, `href="/year/2025"`} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered trophy missing %q:\n%s", fragment, rendered)
		}
	}
}

func TestMonthBarsScaleToBusiestMonth(t *testing.T) {
	bars := monthBars([12]int{0, 1, 40, 20})
	if len(bars) != 12 || bars[0].Label != "Jan" || bars[11].Label != "Dec" {
		t.Fatalf("bars = %#v, want January to December", bars)
	}
	// A month with a visit never vanishes beside a much busier one.
	for i, want := range []string{"0%", "6%", "100%", "50%"} {
		if bars[i].Height != want {
			t.Fatalf("bar %d height = %s, want %s", i, bars[i].Height, want)
		}
	}
	if empty := monthBars([12]int{}); empty[5].Height != "0%" {
		t.Fatalf("empty year bar height = %s, want 0%%", empty[5].Height)
	}
}
