package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/repository"
)

// postRestaurantForm saves a restaurant's edit form the way the page posts it.
func postRestaurantForm(t *testing.T, router http.Handler, token string, restaurant model.Restaurant, closed bool) {
	t.Helper()
	form := url.Values{"restaurant_name": {restaurant.Name}}
	if closed {
		form.Set("closed", "true")
	}
	req := httptest.NewRequest(http.MethodPost, "/restaurants/"+restaurant.ID.String(), strings.NewReader(form.Encode()))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save restaurant = %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRouterClosedRestaurantsLeaveSpinnerAndWantToTryButKeepHistory(t *testing.T) {
	store := spinnerTestStore(t)
	router, token := newAuthenticatedTestRouter(t, store)
	patio := findRestaurant(t, store, "Patio")
	haunt := findRestaurant(t, store, "Old Haunt")

	body := getPublic(router, "/where-to-eat").Body.String()
	if !strings.Contains(body, "2 places in the hat") {
		t.Fatalf("spinner before closing:\n%s", body)
	}

	postRestaurantForm(t, router, token, patio, true)
	postRestaurantForm(t, router, token, haunt, true)

	spinner := getPublic(router, "/where-to-eat?spin=1").Body.String()
	for _, name := range []string{"El Patio Verde", "Old Haunt"} {
		if strings.Contains(spinner, name) {
			t.Fatalf("spinner still offers closed %s:\n%s", name, spinner)
		}
	}
	if !strings.Contains(spinner, "Nothing in the hat yet") || strings.Contains(spinner, "data-spin-result") {
		t.Fatalf("spinner with only closed places:\n%s", spinner)
	}
	if want := getWithSession(router, token, "/want-to-try").Body.String(); strings.Contains(want, "El Patio Verde") {
		t.Fatalf("want to try still lists the closed restaurant:\n%s", want)
	}

	// History keeps the closed restaurant's visits, now with a Closed badge.
	dines := getPublic(router, "/dines").Body.String()
	if !strings.Contains(dines, "Old Haunt") || !strings.Contains(dines, `<span class="badge badge-closed">Closed</span>`) {
		t.Fatalf("dines should keep Old Haunt under a Closed badge:\n%s", dines)
	}
	page := getPublic(router, "/restaurants/"+haunt.ID.String()).Body.String()
	if !strings.Contains(page, `badge-closed">Closed`) {
		t.Fatalf("restaurant page missing Closed badge:\n%s", page)
	}
	search := getWithSession(router, token, "/search?q=Old+Haunt").Body.String()
	if !strings.Contains(search, `<em class="closed-tag">Closed</em>`) {
		t.Fatalf("search missing Closed tag:\n%s", search)
	}

	// The edit form shows the toggle on, and unchecking it reopens the restaurant.
	edit := getWithSession(router, token, "/restaurants/"+haunt.ID.String()+"/edit").Body.String()
	if !strings.Contains(edit, `name="closed" value="true" checked> Permanently closed since`) {
		t.Fatalf("edit form should show Old Haunt closed:\n%s", edit)
	}
	postRestaurantForm(t, router, token, haunt, false)
	if body := getPublic(router, "/where-to-eat").Body.String(); !strings.Contains(body, "1 place in the hat") {
		t.Fatalf("reopened restaurant should be back in the hat:\n%s", body)
	}
}

func findRestaurant(t *testing.T, store *repository.MemoryStore, query string) model.Restaurant {
	t.Helper()
	restaurants, err := store.Restaurants(t.Context(), query)
	if err != nil || len(restaurants) != 1 {
		t.Fatalf("restaurants %q = %#v, err = %v", query, restaurants, err)
	}
	return restaurants[0]
}
