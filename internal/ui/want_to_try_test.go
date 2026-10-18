package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/google/uuid"
)

func wantToTryTestPlaces() []places.Place {
	return []places.Place{
		{ID: "place-deli", DisplayName: places.Name{Text: "Third Street Deli"}, Address: "62 N 3rd St, Dublin, OH", Rating: 4.5, PriceLevel: "PRICE_LEVEL_INEXPENSIVE", Location: places.Location{Latitude: 40.1, Longitude: -83.1}, Types: []string{"sandwich_shop"}},
		{ID: "place-noodle", DisplayName: places.Name{Text: "Lucky Noodle Bar"}, Address: "1 Noodle Way"},
	}
}

func TestRenderPlaceRowsOfferWantToTry(t *testing.T) {
	for _, page := range []string{"nearby", "search"} {
		t.Run(page, func(t *testing.T) {
			var out strings.Builder
			err := Render(&out, page, PageData{
				Authenticated:  true,
				Places:         wantToTryTestPlaces(),
				RequestURI:     "/" + page + "?q=deli",
				WantToTryKeys:  map[string]bool{"place-noodle": true},
				WantToTrySheet: true,
				People:         []model.Person{{ID: uuid.New(), Name: "Caleb"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			body := out.String()
			for _, want := range []string{
				`<a class="want-link" href="/want-to-try/new?address=62&#43;N&#43;3rd&#43;St%2C&#43;Dublin%2C&#43;OH&amp;category=`,
				`google_place_id=place-deli`,
				`google_price_level=1`,
				`google_rating=4.5`,
				`latitude=40.100000`,
				`return_to=%2F` + page + `%3Fq%3Ddeli`,
				`hx-boost="false" data-want-to-try-open data-want-to-try-name="Third Street Deli" data-want-to-try-address="62 N 3rd St, Dublin, OH"`,
				`<span class="want-on">✓ On Want to Try</span>`,
				`<dialog class="want-sheet" id="want-to-try-sheet"`,
				`<option value="everybody">Everybody</option><option value="`,
			} {
				if !strings.Contains(body, want) {
					t.Fatalf("%s page missing %q:\n%s", page, want, body)
				}
			}
			if strings.Count(body, "data-want-to-try-open") != 1 {
				t.Fatalf("only the unlisted place should offer + Want to Try:\n%s", body)
			}
		})
	}
}

func TestRenderPlaceRowsHideWantToTryWhenLoggedOut(t *testing.T) {
	var out strings.Builder
	if err := Render(&out, "nearby", PageData{Places: wantToTryTestPlaces(), WantToTrySheet: true}); err != nil {
		t.Fatal(err)
	}
	if body := out.String(); strings.Contains(body, "+ Want to Try") || strings.Contains(body, "data-want-to-try-open") || strings.Contains(body, "want-to-try-sheet") {
		t.Fatalf("logged-out nearby should not offer + Want to Try:\n%s", body)
	}
}

func TestRenderWantToTryListPickLinkStartsHidden(t *testing.T) {
	note := "Brisket sells out by 7."
	category, city := "BBQ", "Westerville"
	var out strings.Builder
	err := Render(&out, "want-to-try", PageData{WantToTryItems: []model.WantToTryItem{{
		ID:         uuid.New(),
		Restaurant: model.Restaurant{ID: uuid.New(), Name: "Smoke & Pine BBQ", Category: &category, City: &city},
		Note:       &note,
		CreatedAt:  time.Now().Add(-22 * 24 * time.Hour),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, want := range []string{
		`1 place we want to try`,
		`data-want-to-try-name="Smoke &amp; Pine BBQ"`,
		`<span class="want-meta">BBQ · Westerville</span>`,
		`<p class="want-note">Brisket sells out by 7.</p>`,
		`Everybody&#39;s idea · 3 weeks ago`,
		`<p class="want-pick" data-want-to-try-pick-area hidden>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("list missing %q:\n%s", want, body)
		}
	}
}

func TestWantToTryAge(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		ago  time.Duration
		want string
	}{
		{time.Hour, "today"},
		{30 * time.Hour, "yesterday"},
		{5 * 24 * time.Hour, "5 days ago"},
		{8 * 24 * time.Hour, "1 week ago"},
		{15 * 24 * time.Hour, "2 weeks ago"},
		{45 * 24 * time.Hour, "1 month ago"},
		{70 * 24 * time.Hour, "2 months ago"},
		{400 * 24 * time.Hour, "1 year ago"},
		{800 * 24 * time.Hour, "2 years ago"},
	} {
		if got := wantToTryAge(now.Add(-test.ago), now); got != test.want {
			t.Fatalf("age %v = %q, want %q", test.ago, got, test.want)
		}
	}
}

func TestWantToTryPlaceLine(t *testing.T) {
	category, city, address := "Japanese", "Dublin", "1 Noodle Way"
	for _, test := range []struct {
		restaurant model.Restaurant
		want       string
	}{
		{model.Restaurant{Category: &category, City: &city, Address: &address}, "Japanese · Dublin"},
		{model.Restaurant{City: &city, Address: &address}, "Dublin"},
		{model.Restaurant{Address: &address}, "1 Noodle Way"},
		{model.Restaurant{}, ""},
	} {
		if got := wantToTryPlaceLine(test.restaurant); got != test.want {
			t.Fatalf("place line = %q, want %q", got, test.want)
		}
	}
	if got := wantToTryIdea(&model.Person{Name: "Caleb"}); got != "Caleb's idea" {
		t.Fatalf("idea = %q", got)
	}
	if got := wantToTryIdea(nil); got != "Everybody's idea" {
		t.Fatalf("idea = %q", got)
	}
}
