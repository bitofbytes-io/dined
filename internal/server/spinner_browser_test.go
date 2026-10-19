package server

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestSpinnerSpinsToAPickInTheBrowser(t *testing.T) {
	chromePath := chromeExecutableForTest()
	if chromePath == "" {
		t.Skip("Chrome or Chromium executable not found")
	}
	// Serve static/app.js, which runs the result card's reel.
	t.Chdir(filepath.Join("..", ".."))

	router, _ := newAuthenticatedTestRouter(t, spinnerTestStore(t))
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

	var name, again, location, nextName, nextHref string
	var overflow bool
	var watch struct {
		Spun   bool `json:"spun"`
		Linked bool `json:"linked"`
	}
	err := chromedp.Run(browserCtx,
		chromedp.EmulateViewport(390, 844),
		chromedp.Navigate(server.URL+"/where-to-eat"),
		chromedp.WaitVisible(`.spin-form select[name="from"]`, chromedp.ByQuery),
		chromedp.SetValue(`.spin-form select[name="from"]`, "cuisine:Pizza", chromedp.ByQuery),
		chromedp.Click(`.spin-form button`, chromedp.ByQuery),
		chromedp.WaitVisible(`[data-spin-result]`, chromedp.ByQuery),
		chromedp.WaitNotPresent(`[data-spin-result].is-spinning`, chromedp.ByQuery),
		chromedp.Text(`[data-spin-name]`, &name, chromedp.ByQuery),
		chromedp.Text(`.spin-form button`, &again, chromedp.ByQuery),
		chromedp.Location(&location),
		chromedp.Evaluate(`document.documentElement.scrollWidth > window.innerWidth`, &overflow),

		// Anywhere holds two places, so Spin again skips Old Haunt and the
		// reel flashes the other name before the link comes back.
		chromedp.SetValue(`.spin-form select[name="from"]`, "", chromedp.ByQuery),
		chromedp.Evaluate(`window.spinWatch = {spun: false, linked: false};
			new MutationObserver(function () {
				var card = document.querySelector("[data-spin-result].is-spinning");
				if (!card) return;
				window.spinWatch.spun = true;
				if (card.querySelector("[data-spin-name]").hasAttribute("href")) window.spinWatch.linked = true;
			}).observe(document.body, {subtree: true, childList: true, attributes: true, characterData: true}); true`, nil),
		chromedp.Click(`.spin-form button`, chromedp.ByQuery),
		chromedp.Poll(`location.search.includes("last=") && document.querySelector("[data-spin-result][data-spin-reel-done]:not(.is-spinning)") !== null`, nil),
		chromedp.Text(`[data-spin-name]`, &nextName, chromedp.ByQuery),
		chromedp.Evaluate(`document.querySelector("[data-spin-name]").getAttribute("href") || ""`, &nextHref),
		chromedp.Evaluate(`window.spinWatch`, &watch),
	)
	if err != nil {
		t.Fatal(err)
	}
	if name != "Old Haunt" {
		t.Fatalf("spin landed on %q, want Old Haunt, the only pizza place", name)
	}
	if again != "SPIN AGAIN" && again != "Spin again" {
		t.Fatalf("button says %q after a spin", again)
	}
	if want := server.URL + "/where-to-eat?spin=1&from=cuisine%3APizza"; location != want {
		t.Fatalf("location = %q, want %q", location, want)
	}
	if overflow {
		t.Fatal("spinner scrolls sideways at 390px")
	}
	if nextName != "El Patio Verde" {
		t.Fatalf("spin again landed on %q, want El Patio Verde, skipping the last pick", nextName)
	}
	if !watch.Spun || watch.Linked {
		t.Fatalf("reel ran: %v, linked while flashing names: %v; want a reel with no link", watch.Spun, watch.Linked)
	}
	if !strings.HasPrefix(nextHref, "/restaurants/") {
		t.Fatalf("after the reel the pick links to %q, want its restaurant page", nextHref)
	}
}
