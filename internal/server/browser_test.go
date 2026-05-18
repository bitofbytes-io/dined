package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
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

	storeCtx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(storeCtx)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(storeCtx, model.VisitInput{
		RestaurantName: "Amigos",
		VisitedAt:      time.Now(),
		PickerID:       people[0].ID,
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

	router := New(&config.Config{APIToken: "secret"}, store, places.NewClient("")).Router()
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
			return network.SetCookie(middleware.CookieName, "secret").
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
