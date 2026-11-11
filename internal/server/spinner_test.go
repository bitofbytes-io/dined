package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/google/uuid"
)

// spinnerTestStore is the memory store with El Patio Verde on Want to Try
// and Old Haunt, a pizza place rated 8.5 two months ago, as a rested favorite.
// The seeded dines are too recent to count as rested.
func spinnerTestStore(t *testing.T) *repository.MemoryStore {
	t.Helper()
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	patio, err := store.Restaurants(ctx, "Patio")
	if err != nil || len(patio) != 1 {
		t.Fatalf("restaurants = %#v, err = %v", patio, err)
	}
	if _, err := store.SaveWantToTry(ctx, model.WantToTryInput{RestaurantID: &patio[0].ID, SuggestedByID: &people[1].ID, Note: "Salsa night"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Old Haunt",
		Address:        "9 Oven Road",
		City:           "Durham",
		Category:       "Pizza",
		VisitedAt:      time.Now().AddDate(0, 0, -60),
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8, people[1].ID: 9},
	}); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestRouterSpinnerIsPublicAndOffersOneFilterDropdown(t *testing.T) {
	router, _ := newAuthenticatedTestRouter(t, spinnerTestStore(t))

	rec := getPublic(router, "/where-to-eat")
	body := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(body, "Where should we eat?") || !strings.Contains(body, "2 places in the hat") {
		t.Fatalf("spinner = %d:\n%s", rec.Code, body)
	}
	for _, want := range []string{`<select name="from">`, `<option value="cuisine:Mexican"`, `<option value="cuisine:Pizza"`, `<option value="city:Apex"`, `<option value="city:Durham"`, `>Spin</button>`} {
		if !strings.Contains(body, want) {
			t.Fatalf("spinner should offer %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, "data-spin-result") {
		t.Fatalf("spinner should not pick before a spin:\n%s", body)
	}
	if strings.Count(body, "<select") != 1 {
		t.Fatalf("spinner should have one dropdown:\n%s", body)
	}
}

func TestRouterSpinnerPicksFromWantToTryAndRestedFavorites(t *testing.T) {
	router, token := newAuthenticatedTestRouter(t, spinnerTestStore(t))

	// Daniel, Jen and Caleb picked the seeded dines; Aiden never has.
	body := getWithSession(router, token, "/where-to-eat?spin=1&from=city:Apex").Body.String()
	for _, want := range []string{"data-spin-result", "From Want to Try", "El Patio Verde", "Mexican · Apex", "Jen&#39;s idea · added today", "Salsa night",
		"It's <b>Aiden's</b> turn to pick", `<option value="city:Apex" selected>`, ">Spin again</button>", `name="last"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("Apex spin should show %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "Old Haunt") {
		t.Fatalf("Apex spin should leave out Durham:\n%s", body)
	}

	body = getPublic(router, "/where-to-eat?spin=1&from=cuisine:pizza").Body.String()
	for _, want := range []string{"Old favorite", "Old Haunt", "Pizza · Durham", "Rated 8.5 · last here 2 months ago", `<option value="cuisine:Pizza" selected>`} {
		if !strings.Contains(body, want) {
			t.Fatalf("pizza spin should show %q:\n%s", want, body)
		}
	}
}

func TestRouterSpinnerListsPastFavoritesForReadOnlyVisitors(t *testing.T) {
	ctx := context.Background()
	store := spinnerTestStore(t)
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Old Haunt (60 days) is too recent for the list; Long Gone is not.
	if _, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Long Gone",
		Address:        "3 Faraway Lane",
		City:           "Cary",
		Category:       "Thai",
		VisitedAt:      time.Now().AddDate(0, -8, 0),
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 9, people[1].ID: 8},
	}); err != nil {
		t.Fatal(err)
	}
	router, _ := newAuthenticatedTestRouter(t, store)

	body := getPublic(router, "/where-to-eat").Body.String()
	for _, want := range []string{"Haven't been in a while", "Long Gone", "Thai · Cary", "8 months ago", "data-past-favorites", `aria-label="Average rating 8.5"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("where to eat should list %q:\n%s", want, body)
		}
	}
	list := body[strings.Index(body, "data-past-favorites"):]
	list = list[:strings.Index(list, "</ul>")]
	if strings.Contains(list, "Old Haunt") || strings.Contains(list, "<form") || strings.Contains(list, "<button") {
		t.Fatalf("past favorites should leave out recent places and edit controls:\n%s", list)
	}

	body = getPublic(router, "/where-to-eat?from=cuisine:Pizza").Body.String()
	if !strings.Contains(body, "Long Gone") {
		t.Fatalf("the spinner filter should not narrow past favorites:\n%s", body)
	}
}

func TestRouterSpinnerWithoutPastFavoritesOmitsTheSection(t *testing.T) {
	router, _ := newAuthenticatedTestRouter(t, spinnerTestStore(t))

	if body := getPublic(router, "/where-to-eat").Body.String(); strings.Contains(body, "Haven't been in a while") {
		t.Fatalf("no place is old enough, so the section should be absent:\n%s", body)
	}
}

func TestRouterSpinnerWithNothingInTheHat(t *testing.T) {
	router, _ := newAuthenticatedTestRouter(t, repository.NewMemoryStore())

	body := getPublic(router, "/where-to-eat?spin=1").Body.String()
	if !strings.Contains(body, "Nothing in the hat yet") || strings.Contains(body, "<select") || strings.Contains(body, "data-spin-result") {
		t.Fatalf("empty spinner:\n%s", body)
	}
}
