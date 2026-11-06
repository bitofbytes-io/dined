package server

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/chromedp/chromedp"
)

func TestWantToTrySheetSavesWithoutLeavingThePage(t *testing.T) {
	chromePath := chromeExecutableForTest()
	if chromePath == "" {
		t.Skip("Chrome or Chromium executable not found")
	}
	// Serve static/app.js, which runs the save sheet.
	t.Chdir(filepath.Join("..", ".."))

	storeCtx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(storeCtx)
	if err != nil {
		t.Fatal(err)
	}
	hank := hankRestaurant(t, store)
	restaurants, err := store.Restaurants(storeCtx, "Patio")
	if err != nil || len(restaurants) != 1 {
		t.Fatalf("restaurants = %#v, err = %v", restaurants, err)
	}
	if _, err := store.SaveWantToTry(storeCtx, model.WantToTryInput{RestaurantID: &restaurants[0].ID}); err != nil {
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

	page := server.URL + "/restaurants/" + hank.ID.String()
	var sheetName, savedText, location, pickText string
	var sheetOpen, overflow bool
	err = chromedp.Do(browserCtx,
		enableNetwork(),
		chromedp.EmulateViewport(390, 844),
		chromedp.Navigate(server.URL+"/health"),
		setSessionCookie(server.URL, token),
		chromedp.Navigate(page),
		chromedp.WaitVisible(css(`[data-want-to-try-open]`)),
		chromedp.Click(css(`[data-want-to-try-open]`)),
		chromedp.WaitVisible(css(`#want-to-try-sheet[open]`)),
		resultInto(chromedp.Text(css(`[data-want-to-try-sheet-name]`)), &sheetName),
		chromedp.SetValue(css(`#want-to-try-sheet select[name="suggested_by"]`), people[1].ID.String()),
		chromedp.SendKeys(css(`#want-to-try-sheet input[name="note"]`), "Fries for Jen"),
		chromedp.Click(css(`[data-want-to-try-save]`)),
		chromedp.WaitVisible(css(`.want-on`)),
		resultInto(chromedp.TextContent(css(`.want-on`)), &savedText),
		evalInto(`document.getElementById("want-to-try-sheet").open`, &sheetOpen),
		resultInto(chromedp.Location(), &location),

		chromedp.Navigate(server.URL+"/want-to-try"),
		chromedp.WaitVisible(css(`[data-want-to-try-pick]`)),
		chromedp.Click(css(`[data-want-to-try-pick]`)),
		chromedp.WaitVisible(css(`[data-want-to-try-pick-result]`)),
		resultInto(chromedp.Text(css(`[data-want-to-try-pick-result]`)), &pickText),
		evalInto(`document.documentElement.scrollWidth > window.innerWidth`, &overflow),
	)
	if err != nil {
		t.Fatal(err)
	}
	if sheetName != hank.Name {
		t.Fatalf("sheet names %q, want %q", sheetName, hank.Name)
	}
	if savedText != "✓ On Want to Try" || sheetOpen || location != page {
		t.Fatalf("after save: link %q, sheet open %v, location %q; want the page to stay put showing it is listed", savedText, sheetOpen, location)
	}
	items, err := store.WantToTry(storeCtx)
	if err != nil || len(items) != 2 || items[0].Restaurant.ID != hank.ID || items[0].SuggestedBy == nil || items[0].SuggestedBy.Name != "Jen" ||
		items[0].Note == nil || *items[0].Note != "Fries for Jen" {
		t.Fatalf("items = %#v, err = %v; want Jen's idea for Hank's on top", items, err)
	}
	if pickText != "How about "+hank.Name+"?" && pickText != "How about "+restaurants[0].Name+"?" {
		t.Fatalf("pick says %q", pickText)
	}
	if overflow {
		t.Fatal("list scrolls sideways at 390px")
	}
}
