package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/google/uuid"
)

func getWithSession(router http.Handler, token, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// hankRestaurant returns the memory store's seeded, visited Hank's Downtown Diner.
func hankRestaurant(t *testing.T, store *repository.MemoryStore) model.Restaurant {
	t.Helper()
	restaurants, err := store.Restaurants(context.Background(), "Hank")
	if err != nil || len(restaurants) != 1 {
		t.Fatalf("restaurants = %#v, err = %v", restaurants, err)
	}
	return restaurants[0]
}

func TestRouterWantToTrySaveFromRestaurantPage(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	router, token := newAuthenticatedTestRouter(t, store)
	hank := hankRestaurant(t, store)
	page := "/restaurants/" + hank.ID.String()

	if body := getWithSession(router, token, page).Body.String(); !strings.Contains(body, `data-want-to-try-open`) ||
		!strings.Contains(body, `href="/want-to-try/new?restaurant_id=`+hank.ID.String()) || !strings.Contains(body, `id="want-to-try-sheet"`) {
		t.Fatalf("restaurant page should offer + Want to Try with the save sheet:\n%s", body)
	}
	if body := getPublic(router, page).Body.String(); strings.Contains(body, "+ Want to Try") || strings.Contains(body, "want-to-try-sheet") {
		t.Fatalf("logged-out restaurant page should not offer + Want to Try:\n%s", body)
	}

	// The sheet's script asks for a status so the page stays put.
	form := url.Values{"restaurant_id": {hank.ID.String()}, "suggested_by": {people[2].ID.String()}, "note": {"Fries again"}}
	req := httptest.NewRequest(http.MethodPost, "/want-to-try", strings.NewReader(form.Encode()))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Dined-Want-To-Try-Sheet", "1")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("sheet save status = %d, want 204: %s", rec.Code, rec.Body.String())
	}

	if body := getWithSession(router, token, page).Body.String(); !strings.Contains(body, "✓ On Want to Try") || strings.Contains(body, "data-want-to-try-open") {
		t.Fatalf("restaurant page should show it is on the list:\n%s", body)
	}
	list := getPublic(router, "/want-to-try")
	body := list.Body.String()
	if list.Code != http.StatusOK || !strings.Contains(body, "Hank&#39;s Downtown Diner") || !strings.Contains(body, "Fries again") ||
		!strings.Contains(body, "Caleb&#39;s idea · today") || !strings.Contains(body, "American · Raleigh") {
		t.Fatalf("public list = %d:\n%s", list.Code, body)
	}
	if strings.Contains(body, "Log it") || strings.Contains(body, "/delete") {
		t.Fatalf("logged-out list should be read-only:\n%s", body)
	}
	if home := getPublic(router, "/").Body.String(); !strings.Contains(home, `href="/want-to-try">1 place on our Want to Try list →`) {
		t.Fatalf("home should tease the list:\n%s", home)
	}
}

func TestRouterWantToTryFormPageWithoutScript(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	router, token := newAuthenticatedTestRouter(t, store)

	if rec := getPublic(router, "/want-to-try/new?restaurant_name=Deli"); rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/login") {
		t.Fatalf("logged-out form page = %d %q, want a login redirect", rec.Code, rec.Header().Get("Location"))
	}
	page := getWithSession(router, token, "/want-to-try/new?restaurant_name=Third+Street+Deli&address=62+N+3rd+St&google_place_id=place-deli&category=Sandwiches&return_to=%2Fnearby%3Fq%3Ddeli")
	body := page.Body.String()
	if page.Code != http.StatusOK || !strings.Contains(body, "<h1>Third Street Deli</h1>") || !strings.Contains(body, `name="google_place_id" value="place-deli"`) ||
		!strings.Contains(body, `href="/nearby?q=deli">Cancel</a>`) || !strings.Contains(body, `<option value="everybody">Everybody</option>`) {
		t.Fatalf("form page = %d:\n%s", page.Code, body)
	}
	if body := getWithSession(router, token, "/want-to-try/new?restaurant_name=Deli&return_to=//evil.example").Body.String(); !strings.Contains(body, `href="/want-to-try">Cancel</a>`) {
		t.Fatalf("form page should not link Cancel off the site:\n%s", body)
	}

	form := url.Values{"restaurant_name": {"Third Street Deli"}, "address": {"62 N 3rd St"}, "google_place_id": {"place-deli"}, "category": {"Sandwiches"}, "suggested_by": {"everybody"}}
	rec := postVisitForm(t, router, token, "/want-to-try", form)
	location := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(location, "/want-to-try?saved=") {
		t.Fatalf("form save = %d %q", rec.Code, location)
	}
	if body := getPublic(router, location).Body.String(); !strings.Contains(body, "Saved Third Street Deli to Want to Try.") || !strings.Contains(body, "Everybody&#39;s idea") {
		t.Fatalf("list after save:\n%s", body)
	}
	if restaurants, err := store.Restaurants(ctx, "Third Street"); err != nil || len(restaurants) != 1 || restaurants[0].GooglePlaceID == nil {
		t.Fatalf("save should create the restaurant with its place ID: %#v, err = %v", restaurants, err)
	}
}

func TestRouterWantToTryRejectsBadSaves(t *testing.T) {
	store := repository.NewMemoryStore()
	router, token := newAuthenticatedTestRouter(t, store)

	req := httptest.NewRequest(http.MethodPost, "/want-to-try", strings.NewReader("restaurant_name=Sneaky"))
	setSameOrigin(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("logged-out save = %d, want 401", rec.Code)
	}

	for _, test := range []struct {
		form   url.Values
		status int
		text   string
	}{
		{url.Values{"restaurant_name": {"Deli"}, "suggested_by": {"nobody"}}, http.StatusBadRequest, "Whose idea? must be Everybody or one of the family."},
		{url.Values{"restaurant_name": {"Deli"}, "suggested_by": {uuid.NewString()}}, http.StatusBadRequest, "Whose idea? must be Everybody or one of the family."},
		{url.Values{"restaurant_id": {uuid.NewString()}}, http.StatusNotFound, "Restaurant not found"},
		{url.Values{"restaurant_name": {"Deli"}, "note": {strings.Repeat("x", model.MaxWantToTryNoteLength+1)}}, http.StatusBadRequest, "note must be 500 characters or fewer"},
	} {
		rec := postVisitForm(t, router, token, "/want-to-try", test.form)
		if rec.Code != test.status || !strings.Contains(rec.Body.String(), test.text) {
			t.Fatalf("save %v = %d, want %d with %q:\n%s", test.form, rec.Code, test.status, test.text, rec.Body.String())
		}
	}
	if items, err := store.WantToTry(context.Background()); err != nil || len(items) != 0 {
		t.Fatalf("bad saves listed %#v, err = %v", items, err)
	}
}

func TestRouterWantToTryLogItClearsTheItem(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	router, token := newAuthenticatedTestRouter(t, store)
	priceLevel := 3
	id, err := store.SaveWantToTry(ctx, model.WantToTryInput{
		RestaurantName: "Lucky Noodle Bar",
		Address:        "1 Noodle Way",
		Category:       "Japanese",
		GoogleMetadata: model.GoogleRestaurantMetadata{GooglePriceLevel: &priceLevel},
		SuggestedByID:  &people[2].ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.WantToTryItem(ctx, *id)
	if err != nil {
		t.Fatal(err)
	}

	list := getWithSession(router, token, "/want-to-try").Body.String()
	if !strings.Contains(list, `href="/log?want_to_try=`+id.String()+`">Log it</a>`) || !strings.Contains(list, `action="/want-to-try/`+id.String()+`/delete"`) {
		t.Fatalf("list should offer Log it and Remove:\n%s", list)
	}

	logPage := getWithSession(router, token, "/log?want_to_try="+id.String()).Body.String()
	for _, want := range []string{
		"From Want to Try · Caleb&#39;s idea",
		`name="want_to_try" value="` + id.String() + `"`,
		`name="restaurant_id" value="` + item.Restaurant.ID.String() + `"`,
		`name="restaurant_name" list="restaurant-options" placeholder="Search or add restaurant" value="Lucky Noodle Bar"`,
		`name="address" placeholder="Optional" value="1 Noodle Way"`,
		`<option selected>Japanese</option>`,
		`name="picker_id" value="` + people[2].ID.String() + `" checked>`,
		`<option value="3" selected>$$$</option>`,
	} {
		if !strings.Contains(logPage, want) {
			t.Fatalf("log page missing %q:\n%s", want, logPage)
		}
	}

	form := pickForm(people, "Lucky Noodle Bar", people[2].ID.String())
	form.Set("restaurant_id", item.Restaurant.ID.String())
	form.Set("address", "1 Noodle Way")
	form.Set("want_to_try", id.String())
	rec := postVisitForm(t, router, token, "/visits", form)
	if location := rec.Header().Get("Location"); rec.Code != http.StatusSeeOther || location != "/want-to-try?logged="+id.String() {
		t.Fatalf("log it = %d %q, want a redirect back to the list", rec.Code, location)
	}
	after := getWithSession(router, token, "/want-to-try?logged="+id.String()).Body.String()
	if !strings.Contains(after, "Logged Lucky Noodle Bar. It&#39;s off the Want to Try list.") || strings.Contains(after, "data-want-to-try-row") {
		t.Fatalf("list after logging:\n%s", after)
	}
	if !strings.Contains(after, "Find a place") {
		t.Fatalf("empty list should offer Find a place:\n%s", after)
	}
	if home := getPublic(router, "/").Body.String(); strings.Contains(home, "want-teaser") {
		t.Fatalf("home should hide the teaser when the list is empty:\n%s", home)
	}

	// A plain dine (no want_to_try) still lands on /dines.
	rec = postVisitForm(t, router, token, "/visits", pickForm(people, "Somewhere Else", "everybody"))
	if location := rec.Header().Get("Location"); !strings.HasPrefix(location, "/dines") {
		t.Fatalf("plain dine redirect = %q, want /dines", location)
	}
}

func TestRouterWantToTryRemove(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	router, token := newAuthenticatedTestRouter(t, store)
	hank := hankRestaurant(t, store)
	id, err := store.SaveWantToTry(ctx, model.WantToTryInput{RestaurantID: &hank.ID})
	if err != nil {
		t.Fatal(err)
	}
	if rec := postVisitForm(t, router, token, "/want-to-try/not-an-id/delete", url.Values{}); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id delete = %d, want 400", rec.Code)
	}
	rec := postVisitForm(t, router, token, "/want-to-try/"+id.String()+"/delete", url.Values{})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/want-to-try" {
		t.Fatalf("delete = %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if items, err := store.WantToTry(ctx); err != nil || len(items) != 0 {
		t.Fatalf("items after delete = %#v, err = %v", items, err)
	}
	if restaurant, err := store.Restaurant(ctx, hank.ID); err != nil || restaurant == nil {
		t.Fatalf("removing from the list should keep the restaurant: %#v, err = %v", restaurant, err)
	}
	if body := getPublic(router, "/want-to-try").Body.String(); !strings.Contains(body, "Nothing on the list yet.") || strings.Contains(body, "Find a place") {
		t.Fatalf("logged-out empty list:\n%s", body)
	}
}
