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

	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/chromedp/cdproto/network"
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
	err = chromedp.Run(browserCtx,
		network.Enable(),
		chromedp.Navigate(server.URL+"/health"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetCookie(middleware.CookieName, token).
				WithURL(server.URL).
				WithPath("/").
				Do(ctx)
		}),
		chromedp.Navigate(server.URL+"/search?q=Amigos"),
		chromedp.WaitVisible(`.history-remove-button`, chromedp.ByQuery),
		chromedp.Click(`.history-remove-button`, chromedp.ByQuery),
		chromedp.WaitVisible(`#delete-dine-modal[open]`, chromedp.ByQuery),
		chromedp.Click(`#delete-dine-cancel-button`, chromedp.ByQuery),
		chromedp.Sleep(150*time.Millisecond),
		chromedp.Evaluate(`document.getElementById("delete-dine-modal").open`, &dialogOpen),
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

func TestHomeRecentDinesFitInsideDesktopBoothScene(t *testing.T) {
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
	now := time.Now()
	longNote := strings.Repeat("Loved the noodles, service, desserts, and the table wanted to come back for another family dinner. ", 18)
	for i, name := range []string{"Tall Booth Test One", "Tall Booth Test Two", "Tall Booth Test Three"} {
		_, err := store.CreateVisit(storeCtx, model.VisitInput{
			RestaurantName: name,
			Address:        "123 Long Menu Lane, Raleigh, NC 27601, USA",
			VisitedAt:      now.Add(-time.Duration(i) * time.Hour),
			PickerID:       &people[i%len(people)].ID,
			PriceLevel:     2,
			Notes:          longNote,
			Ratings: map[uuid.UUID]float64{
				people[0].ID: 8,
				people[1].ID: 8.5,
				people[2].ID: 7.5,
				people[3].ID: 9,
			},
			NewTag: "Would Return",
			Photos: []model.VisitPhotoInput{
				{DataURI: "data:image/jpeg;base64,aGVsbG8="},
				{DataURI: "data:image/jpeg;base64,dGFjbw=="},
				{DataURI: "data:image/jpeg;base64,cmljZQ=="},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
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

	var metrics struct {
		VisibleCards   int     `json:"visibleCards"`
		SceneBottom    float64 `json:"sceneBottom"`
		SceneHeight    float64 `json:"sceneHeight"`
		MaxCardBottom  float64 `json:"maxCardBottom"`
		ActionBottom   float64 `json:"actionBottom"`
		RecentTop      float64 `json:"recentTop"`
		RecentPosition string  `json:"recentPosition"`
	}
	err = chromedp.Run(browserCtx,
		network.Enable(),
		chromedp.EmulateViewport(1440, 900),
		chromedp.Navigate(server.URL+"/health"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetCookie(middleware.CookieName, token).
				WithURL(server.URL).
				WithPath("/").
				Do(ctx)
		}),
		chromedp.Navigate(server.URL+"/"),
		chromedp.WaitVisible(`.booth-recent .visit-card`, chromedp.ByQuery),
		chromedp.Evaluate(`(() => {
			const scene = document.querySelector(".booth-scene").getBoundingClientRect();
			const action = document.querySelector(".booth-layer-action").getBoundingClientRect();
			const recent = document.querySelector(".booth-recent");
			const cards = Array.from(document.querySelectorAll(".booth-recent .visit-card"))
				.filter((card) => getComputedStyle(card).display !== "none");
			return {
				visibleCards: cards.length,
				sceneBottom: scene.bottom,
				sceneHeight: scene.height,
				maxCardBottom: Math.max(...cards.map((card) => card.getBoundingClientRect().bottom)),
				actionBottom: action.bottom,
				recentTop: recent.getBoundingClientRect().top,
				recentPosition: getComputedStyle(recent).position
			};
		})()`, &metrics),
	)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.VisibleCards != 3 {
		t.Fatalf("visible recent cards = %d, want 3", metrics.VisibleCards)
	}
	if metrics.RecentPosition == "absolute" {
		t.Fatal("recent dines should participate in desktop booth layout, but position is absolute")
	}
	if metrics.RecentTop < metrics.ActionBottom {
		t.Fatalf("recent dines overlap search action: recent top %.1f, action bottom %.1f", metrics.RecentTop, metrics.ActionBottom)
	}
	if metrics.SceneHeight <= 760 {
		t.Fatalf("booth scene did not expand for tall cards: height %.1f", metrics.SceneHeight)
	}
	if metrics.MaxCardBottom > metrics.SceneBottom+1 {
		t.Fatalf("recent cards overflow booth scene: card bottom %.1f, scene bottom %.1f", metrics.MaxCardBottom, metrics.SceneBottom)
	}
}

func TestEditVisitPhotoAddTileIsFirstAndFullSizeOnMobile(t *testing.T) {
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
	visitID, err := store.CreateVisit(storeCtx, model.VisitInput{
		RestaurantName: "Long Wait",
		VisitedAt:      time.Now(),
		PickerID:       &people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
		Photos: []model.VisitPhotoInput{
			{DataURI: "data:image/jpeg;base64,aGVsbG8="},
			{DataURI: "data:image/jpeg;base64,dGFjbw=="},
			{DataURI: "data:image/jpeg;base64,cmljZQ=="},
		},
	})
	if err != nil {
		t.Fatal(err)
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

	var metrics struct {
		AddLeft        float64 `json:"addLeft"`
		AddWidth       float64 `json:"addWidth"`
		AddHeight      float64 `json:"addHeight"`
		FirstPhotoLeft float64 `json:"firstPhotoLeft"`
	}
	err = chromedp.Run(browserCtx,
		network.Enable(),
		chromedp.EmulateViewport(390, 844),
		chromedp.Navigate(server.URL+"/health"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetCookie(middleware.CookieName, token).
				WithURL(server.URL).
				WithPath("/").
				Do(ctx)
		}),
		chromedp.Navigate(server.URL+"/visits/"+visitID.String()+"/edit"),
		chromedp.WaitVisible(`[data-photo-add]`, chromedp.ByQuery),
		chromedp.Evaluate(`(() => {
			const add = document.querySelector("[data-photo-add]").getBoundingClientRect();
			const firstPhoto = document.querySelector("[data-photo-item]").getBoundingClientRect();
			return {
				addLeft: add.left,
				addWidth: add.width,
				addHeight: add.height,
				firstPhotoLeft: firstPhoto.left
			};
		})()`, &metrics),
	)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.AddWidth < 90 || metrics.AddHeight < 65 {
		t.Fatalf("add-photo tile too small on mobile: %.1fx%.1f", metrics.AddWidth, metrics.AddHeight)
	}
	if metrics.AddLeft >= metrics.FirstPhotoLeft {
		t.Fatalf("add-photo tile should be before photos on mobile: add left %.1f, first photo left %.1f", metrics.AddLeft, metrics.FirstPhotoLeft)
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
	err = chromedp.Run(browserCtx,
		network.Enable(),
		chromedp.Navigate(server.URL+"/health"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return network.SetCookie(middleware.CookieName, token).WithURL(server.URL).WithPath("/").Do(ctx)
		}),
		chromedp.Navigate(logURL),
		chromedp.WaitVisible(`[data-log-form]`, chromedp.ByQuery),
		chromedp.Evaluate(helpers, nil),
		// Pick Corner Noodles from the list, then clear Google's address so it matches.
		chromedp.Evaluate(`dinedType("restaurant_name", "Corner Noodles"); dinedType("address", ""); dinedFields()`, &picked),
		// Typing a name that is not listed un-picks it and restores Hank's place.
		chromedp.Evaluate(`dinedType("restaurant_name", "Corner Noodles Annex"); dinedFields()`, &unpicked),
		// Pick a restaurant with its own place, then switch to Corner Noodles.
		chromedp.Evaluate(`dinedType("restaurant_name", "El Patio Verde"); dinedFields()`, &patio),
		chromedp.Evaluate(`dinedType("restaurant_name", "Corner Noodles"); dinedType("address", ""); dinedFields()`, &backToCorner),
		chromedp.Evaluate(`dinedType("rating_`+people[0].ID.String()+`", "7")`, nil),
		chromedp.Click(`[data-log-submit]`, chromedp.ByQuery),
		chromedp.Poll(`location.pathname === "/dines"`, nil),
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
