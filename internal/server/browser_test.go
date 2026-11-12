package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/chromedp/chromedp"
	"github.com/google/uuid"
)

func TestSearchRemoveCancelDoesNotDeleteRestaurant(t *testing.T) {
	chromePath := chromeExecutableForTest()
	if chromePath == "" {
		t.Skip("Chrome or Chromium executable not found")
	}
	// Serve static/app.js, which handles the delete dialog.
	t.Chdir(filepath.Join("..", ".."))

	storeCtx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(storeCtx)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(storeCtx, model.VisitInput{
		RestaurantName: "Amigos",
		VisitedAt:      time.Now(),
		PickerID:       &people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	restaurants, err := store.Restaurants(storeCtx, "Amigos")
	if err != nil {
		t.Fatal(err)
	}
	if len(restaurants) != 1 {
		t.Fatalf("expected one Amigos restaurant, got %d", len(restaurants))
	}
	restaurantID := restaurants[0].ID
	if err := store.DeleteVisit(storeCtx, *visitID); err != nil {
		t.Fatal(err)
	}

	router, token := newAuthenticatedTestRouter(t, store)
	var deletePosts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/restaurants/"+restaurantID.String()+"/delete" {
			deletePosts.Add(1)
		}
		router.ServeHTTP(w, r)
	}))
	defer server.Close()

	options := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.Headless,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.DisableGPU,
		chromedp.Flag("disable-dev-shm-usage", true),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), options...)
	defer cancelAlloc()

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx, chromedp.WithLogf(func(string, ...any) {}))
	defer cancelBrowser()
	browserCtx, cancelTimeout := context.WithTimeout(browserCtx, 20*time.Second)
	defer cancelTimeout()

	var dialogOpen bool
	err = chromedp.Do(browserCtx,
		enableNetwork(),
		chromedp.Navigate(server.URL+"/health"),
		setSessionCookie(server.URL, token),
		chromedp.Navigate(server.URL+"/search?q=Amigos"),
		chromedp.WaitVisible(css(`.history-remove-button`)),
		chromedp.Click(css(`.history-remove-button`)),
		chromedp.WaitVisible(css(`#delete-dine-modal[open]`)),
		chromedp.Click(css(`#delete-dine-cancel-button`)),
		chromedp.Sleep(150*time.Millisecond),
		evalInto(`document.getElementById("delete-dine-modal").open`, &dialogOpen),
	)
	if err != nil {
		t.Fatal(err)
	}
	if dialogOpen {
		t.Fatal("delete dialog stayed open after cancel")
	}
	if got := deletePosts.Load(); got != 0 {
		t.Fatalf("cancel posted to delete endpoint %d time(s), want 0", got)
	}
	remaining, err := store.Restaurants(storeCtx, "Amigos")
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 {
		t.Fatalf("cancel deleted search restaurant, remaining restaurants = %#v", remaining)
	}
}

func TestDinesFilterDialogAppliesChosenFilters(t *testing.T) {
	chromePath := chromeExecutableForTest()
	if chromePath == "" {
		t.Skip("Chrome or Chromium executable not found")
	}
	// Serve static/app.js, which opens the dialog and submits the filters.
	t.Chdir(filepath.Join("..", ".."))
	router, _ := newAuthenticatedTestRouter(t, repository.NewMemoryStore())
	server := httptest.NewServer(router)
	defer server.Close()

	options := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.Headless,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.DisableGPU,
		chromedp.Flag("disable-dev-shm-usage", true),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), options...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx, chromedp.WithLogf(func(string, ...any) {}))
	defer cancelBrowser()
	browserCtx, cancelTimeout := context.WithTimeout(browserCtx, 20*time.Second)
	defer cancelTimeout()

	var closedAfterClose bool
	var search, status string
	var cards int
	err := chromedp.Do(browserCtx,
		chromedp.Navigate(server.URL+"/dines"),
		chromedp.WaitVisible(css(`[data-dines-filter-open]`)),
		chromedp.Click(css(`[data-dines-filter-open]`)),
		chromedp.WaitVisible(css(`#dines-filter-dialog[open]`)),
		chromedp.Click(css(`[data-dines-filter-close]`)),
		evalInto(`!document.getElementById("dines-filter-dialog").open`, &closedAfterClose),
		chromedp.Click(css(`[data-dines-filter-open]`)),
		chromedp.WaitVisible(css(`#dines-filter-dialog[open]`)),
		chromedp.SetValue(css(`select[name="cuisine"]`), "Mexican"),
		chromedp.SetValue(css(`select[name="picker"]`), "Jen"),
		chromedp.Click(css(`.dines-filter-actions button`)),
		chromedp.WaitVisible(css(`.dines-filter-summary`)),
		evalInto(`window.location.search`, &search),
		evalInto(`document.querySelector(".dines-filter-status").textContent`, &status),
		evalInto(`document.querySelectorAll(".visit-card").length`, &cards),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !closedAfterClose {
		t.Fatal("filter dialog stayed open after Close")
	}
	// Filters left at Any stay out of the URL.
	if search != "?cuisine=Mexican&picker=Jen" {
		t.Fatalf("filtered URL search = %q, want only the chosen filters", search)
	}
	if cards != 1 || !strings.Contains(status, "Mexican · Jen's picks") || !strings.Contains(status, "1 of 3 dines") {
		t.Fatalf("filtered page shows %d cards and status %q", cards, status)
	}
}

// Picking an existing restaurant on a log form opened from another
// restaurant's Google result must not carry that place's ID, details or
// category along.
func TestLogFormPickingRestaurantReplacesOtherGooglePlace(t *testing.T) {
	chromePath := chromeExecutableForTest()
	if chromePath == "" {
		t.Skip("Chrome or Chromium executable not found")
	}
	t.Chdir(filepath.Join("..", ".."))

	storeCtx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(storeCtx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVisit(storeCtx, model.VisitInput{
		RestaurantName: "Corner Noodles",
		VisitedAt:      time.Now().Add(-48 * time.Hour),
		PickerID:       &people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	}); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, name := range []string{"Corner Noodles", "El Patio Verde"} {
		restaurants, err := store.Restaurants(storeCtx, name)
		if err != nil || len(restaurants) != 1 {
			t.Fatalf("find %s: %#v, %v", name, restaurants, err)
		}
		ids[name] = restaurants[0].ID.String()
	}

	router, token := newAuthenticatedTestRouter(t, store)
	server := httptest.NewServer(router)
	defer server.Close()

	options := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.Headless,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.DisableGPU,
		chromedp.Flag("disable-dev-shm-usage", true),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), options...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx, chromedp.WithLogf(func(string, ...any) {}))
	defer cancelBrowser()
	browserCtx, cancelTimeout := context.WithTimeout(browserCtx, 20*time.Second)
	defer cancelTimeout()

	// The "Log this dine" link for Hank's Downtown Diner's Google result.
	logURL := server.URL + "/log?restaurant_name=Hank%27s+Downtown+Diner&address=101+Main+Street&city=Raleigh" +
		"&google_place_id=demo-hanks&phone=919-555-0100&website=https%3A%2F%2Fhanks.example&google_rating=4.3" +
		"&google_price_level=2&latitude=35.779600&longitude=-78.638200&category=American&price_level=2"
	// dinedType sets a field as a datalist pick does: the whole value, then an input event.
	const helpers = `window.dinedType = (name, value) => {
		const field = document.querySelector("[name='" + name + "']");
		field.value = value;
		field.dispatchEvent(new Event("input", { bubbles: true }));
	};
	window.dinedFields = () => Object.fromEntries(["restaurant_id", "google_place_id", "phone", "website", "latitude", "longitude", "google_rating", "google_price_level", "category"]
		.map((name) => [name, document.querySelector("[name='" + name + "']").value]));`
	type fields struct {
		RestaurantID     string `json:"restaurant_id"`
		GooglePlaceID    string `json:"google_place_id"`
		Phone            string `json:"phone"`
		Website          string `json:"website"`
		Latitude         string `json:"latitude"`
		Longitude        string `json:"longitude"`
		GoogleRating     string `json:"google_rating"`
		GooglePriceLevel string `json:"google_price_level"`
		Category         string `json:"category"`
	}
	hanks := fields{GooglePlaceID: "demo-hanks", Phone: "919-555-0100", Website: "https://hanks.example", Latitude: "35.779600", Longitude: "-78.638200", GoogleRating: "4.3", GooglePriceLevel: "2", Category: "American"}
	var picked, unpicked, patio, backToCorner fields
	err = chromedp.Do(browserCtx,
		enableNetwork(),
		chromedp.Navigate(server.URL+"/health"),
		setSessionCookie(server.URL, token),
		chromedp.Navigate(logURL),
		chromedp.WaitVisible(css(`[data-log-form]`)),
		chromedp.Evaluate[chromedp.Void](helpers),
		// Pick Corner Noodles from the list, then clear Google's address so it matches.
		evalInto(`dinedType("restaurant_name", "Corner Noodles"); dinedType("address", ""); dinedFields()`, &picked),
		// Typing a name that is not listed un-picks it and restores Hank's place.
		evalInto(`dinedType("restaurant_name", "Corner Noodles Annex"); dinedFields()`, &unpicked),
		// Pick a restaurant with its own place, then switch to Corner Noodles.
		evalInto(`dinedType("restaurant_name", "El Patio Verde"); dinedFields()`, &patio),
		evalInto(`dinedType("restaurant_name", "Corner Noodles"); dinedType("address", ""); dinedFields()`, &backToCorner),
		chromedp.Evaluate[chromedp.Void](`dinedType("rating_`+people[0].ID.String()+`", "7")`),
		chromedp.Click(css(`[data-log-submit]`)),
		chromedp.Poll[chromedp.Void](`location.pathname === "/dines"`),
	)
	if err != nil {
		t.Fatal(err)
	}

	if want := (fields{RestaurantID: ids["Corner Noodles"]}); picked != want {
		t.Fatalf("after picking Corner Noodles: %#v, want %#v", picked, want)
	}
	if unpicked != hanks {
		t.Fatalf("after un-picking: %#v, want Hank's Google place restored %#v", unpicked, hanks)
	}
	if want := (fields{RestaurantID: ids["El Patio Verde"], GooglePlaceID: "demo-patio", Category: "Mexican"}); patio != want {
		t.Fatalf("after picking El Patio Verde: %#v, want %#v", patio, want)
	}
	if want := (fields{RestaurantID: ids["Corner Noodles"]}); backToCorner != want {
		t.Fatalf("after switching to Corner Noodles: %#v, want %#v", backToCorner, want)
	}

	visits, err := store.Visits(storeCtx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(visits) != 1 || visits[0].Restaurant.Name != "Corner Noodles" {
		t.Fatalf("newest visit = %#v, want one at Corner Noodles", visits)
	}
	if corner := visits[0].Restaurant; corner.GooglePlaceID != nil || corner.Phone != nil || corner.Latitude != nil {
		t.Fatalf("Corner Noodles gained Google details: %#v", corner)
	}
}

func chromeExecutableForTest() string {
	if path := os.Getenv("CHROME_BIN"); isExecutable(path) {
		return path
	}
	if path := os.Getenv("CHROMIUM_BIN"); isExecutable(path) {
		return path
	}

	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = append(candidates,
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Google Chrome for Testing.app/Contents/MacOS/Google Chrome for Testing",
		)
	case "linux":
		candidates = append(candidates,
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
		)
	}
	for _, candidate := range candidates {
		if isExecutable(candidate) {
			return candidate
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(home, ".cache", "puppeteer", "chrome", "*", "chrome-*", "Google Chrome for Testing.app", "Contents", "MacOS", "Google Chrome for Testing"))
	sort.Strings(matches)
	for i := len(matches) - 1; i >= 0; i-- {
		if isExecutable(matches[i]) {
			return matches[i]
		}
	}
	return ""
}

func isExecutable(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Mode()&0o111 != 0
}
