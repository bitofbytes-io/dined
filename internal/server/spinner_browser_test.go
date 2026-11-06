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
	err := chromedp.Do(browserCtx,
		chromedp.EmulateViewport(390, 844),
		chromedp.Navigate(server.URL+"/where-to-eat"),
		chromedp.WaitVisible(css(`.spin-form select[name="from"]`)),
		chromedp.SetValue(css(`.spin-form select[name="from"]`), "cuisine:Pizza"),
		chromedp.Evaluate[chromedp.Void](`window.firstSpinSettled = false;
			document.addEventListener("htmx:afterSettle", function () { window.firstSpinSettled = true; }, {once: true});`),
		chromedp.Click(css(`.spin-form button`)),
		chromedp.WaitVisible(css(`[data-spin-result]`)),
		chromedp.WaitNotPresent(css(`[data-spin-result].is-spinning`)),
		// The result is visible before HTMX finishes initializing the replacement form.
		chromedp.Poll[chromedp.Void](`window.firstSpinSettled === true`),
		resultInto(chromedp.Text(css(`[data-spin-name]`)), &name),
		resultInto(chromedp.Text(css(`.spin-form button`)), &again),
		resultInto(chromedp.Location(), &location),
		evalInto(`document.documentElement.scrollWidth > window.innerWidth`, &overflow),

		// Anywhere holds two places, so Spin again skips Old Haunt and the
		// reel flashes the other name before the link comes back.
		chromedp.SetValue(css(`.spin-form select[name="from"]`), ""),
		chromedp.Evaluate[chromedp.Void](`window.spinWatch = {spun: false, linked: false};
			new MutationObserver(function () {
				var card = document.querySelector("[data-spin-result].is-spinning");
				if (!card) return;
				window.spinWatch.spun = true;
				if (card.querySelector("[data-spin-name]").hasAttribute("href")) window.spinWatch.linked = true;
			}).observe(document.body, {subtree: true, childList: true, attributes: true, characterData: true}); true`),
		chromedp.Click(css(`.spin-form button`)),
		chromedp.Poll[chromedp.Void](`location.search.includes("last=") && document.querySelector("[data-spin-result][data-spin-reel-done]:not(.is-spinning)") !== null`),
		resultInto(chromedp.Text(css(`[data-spin-name]`)), &nextName),
		evalInto(`document.querySelector("[data-spin-name]").getAttribute("href") || ""`, &nextHref),
		evalInto(`window.spinWatch`, &watch),
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
