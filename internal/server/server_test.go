package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/google/uuid"
)

func TestRouterDeletesUnvisitedRestaurantAndPreservesSearch(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Amigos",
		VisitedAt:      time.Now(),
		PickerID:       people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	restaurants, err := store.Restaurants(ctx, "Amigos")
	if err != nil {
		t.Fatal(err)
	}
	if len(restaurants) != 1 {
		t.Fatalf("expected one Amigos restaurant, got %d", len(restaurants))
	}
	if err := store.DeleteVisit(ctx, *visitID); err != nil {
		t.Fatal(err)
	}

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/restaurants/"+restaurants[0].ID.String()+"/delete", strings.NewReader("q=Amigos"))
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "/search?q=Amigos" {
		t.Fatalf("redirect location = %q, want /search?q=Amigos", location)
	}
	remaining, err := store.Restaurants(ctx, "Amigos")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected Amigos to be deleted, got %#v", remaining)
	}
}

func TestRouterRefusesVisitedRestaurantDelete(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	restaurants, err := store.Restaurants(ctx, "Hank")
	if err != nil {
		t.Fatal(err)
	}
	if len(restaurants) != 1 {
		t.Fatalf("expected one Hank restaurant, got %d", len(restaurants))
	}

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/restaurants/"+restaurants[0].ID.String()+"/delete", strings.NewReader("q=Hank"))
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("got status %d, want %d", rec.Code, http.StatusConflict)
	}
	remaining, err := store.Restaurants(ctx, "Hank")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 {
		t.Fatalf("expected visited restaurant to remain, got %#v", remaining)
	}
}

func TestRouterCreateVisitWithoutRatingPreservesPostedForm(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tags, err := store.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Visits(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	form := url.Values{}
	form.Set("restaurant_name", "Tupelo Honey")
	form.Set("address", "123 Main St")
	form.Set("city", "Apex")
	form.Set("google_place_id", "place-1")
	form.Set("category", "American")
	form.Set("visited_at", "2026-05-17T20:50")
	form.Set("picker_id", people[1].ID.String())
	form.Set("price_level", "3")
	form.Set("notes", "Dinner notes")
	form.Set("new_tag", "Patio")
	form.Set("is_chain", "true")
	form.Add("tag_id", tags[0].ID.String())

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/visits", strings.NewReader(form.Encode()))
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	rendered := rec.Body.String()
	for _, fragment := range []string{
		"at least one rating is required",
		`name="restaurant_name" list="restaurant-options" placeholder="Search or add restaurant" value="Tupelo Honey" required`,
		`name="address" placeholder="Optional" value="123 Main St"`,
		`name="city" value="Apex"`,
		`name="google_place_id" placeholder="Optional" value="place-1"`,
		`<option selected>American</option>`,
		`name="visited_at" value="2026-05-17T20:50" required`,
		`value="` + people[1].ID.String() + `" selected>` + people[1].Name + `</option>`,
		`value="3" selected>$$$</option>`,
		`name="tag_id" value="` + tags[0].ID.String() + `" checked`,
		`name="new_tag" placeholder="Great fries" value="Patio"`,
		`Dinner notes</textarea>`,
		`name="is_chain" value="true" checked`,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("response missing %q:\n%s", fragment, rendered)
		}
	}

	after, err := store.Visits(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("visit count = %d, want %d", len(after), len(before))
	}
}

func TestRouterCreateVisitAcceptsZeroRating(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Visits(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	form := url.Values{}
	form.Set("restaurant_name", "Zero Star Diner")
	form.Set("visited_at", "2026-05-17T20:50")
	form.Set("picker_id", people[0].ID.String())
	form.Set("price_level", "2")
	form.Set("rating_"+people[0].ID.String(), "0")

	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodPost, "/visits", strings.NewReader(form.Encode()))
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("got status %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	after, err := store.Visits(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)+1 {
		t.Fatalf("visit count = %d, want %d", len(after), len(before)+1)
	}
	if after[0].Restaurant.Name != "Zero Star Diner" {
		t.Fatalf("newest restaurant = %q, want Zero Star Diner", after[0].Restaurant.Name)
	}
	if len(after[0].Ratings) != 1 || after[0].Ratings[0].Score != 0 {
		t.Fatalf("newest ratings = %#v, want one zero rating", after[0].Ratings)
	}
}
