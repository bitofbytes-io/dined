package server

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/bitofbytes-io/dined/internal/testutil"
	"github.com/chromedp/chromedp"
	"github.com/google/uuid"
)

const photoPendingMessage = "Photos are still being prepared. Try again in a moment."

// While a picked photo is still compressing, no photo form may send a request,
// including the hx-boost requests htmx issues from its own submit handler
// before any document listener runs. The save button stays disabled until the
// photo lands, and the saved dine then carries it.
func TestPhotoFormsBlockSavingWhileCompressing(t *testing.T) {
	chromePath := chromeExecutableForTest()
	if chromePath == "" {
		t.Skip("Chrome or Chromium executable not found")
	}
	t.Chdir(filepath.Join("..", ".."))

	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, visitID := seedSlowPhotoVisit(t, store)
	photoPath := writePickPhoto(t)

	router, token := newAuthenticatedTestRouter(t, store)
	server := httptest.NewServer(router)
	defer server.Close()

	browserCtx, cancelBrowser := newPhotoBrowserContext(t, chromePath)
	defer cancelBrowser()

	// holdImages keeps picked photos "compressing" until releaseImages runs, and
	// counts every htmx request and native submit that goes out meanwhile.
	const helpers = `(() => {
		window.__sent = 0;
		document.addEventListener("htmx:beforeSend", () => { window.__sent += 1; });
		const setSrc = Object.getOwnPropertyDescriptor(HTMLImageElement.prototype, "src").set;
		const RealImage = window.Image;
		const held = [];
		window.Image = function () {
			const img = new RealImage();
			Object.defineProperty(img, "src", { configurable: true, set(value) { held.push(() => setSrc.call(img, value)); } });
			return img;
		};
		window.releaseImages = () => { while (held.length) held.shift()(); };
	})()`
	// attempt tries every way of submitting the first photo form on the page.
	const attempt = `(() => {
		const form = document.querySelector("[data-photo-uploader]").closest("form");
		form.requestSubmit();
		const button = form.querySelector("button[type=submit], button:not([type])");
		if (button && !button.disabled) button.click();
		return window.__sent;
	})()`

	var boosted bool
	var sentEdit, sentLog int
	var editDisabled, editError, logError string
	actions := append(openEditPageByBoost(server.URL, token, visitID),
		evalInto(`window.__boostMarker === true`, &boosted),
		chromedp.Evaluate[chromedp.Void](helpers),
		chromedp.SetUploadFiles(css(`[data-photo-input]`), []string{photoPath}),
		chromedp.Poll[chromedp.Void](`document.querySelector("[data-photo-submit]").disabled`),
		evalInto(attempt, &sentEdit),
		evalInto(`document.querySelector("[data-photo-error]").textContent`, &editError),
		evalInto(`String(document.querySelector("[data-photo-submit]").disabled)`, &editDisabled),
		// Once the photo lands the save button comes back and the save carries it.
		chromedp.Evaluate[chromedp.Void](`window.releaseImages()`),
		chromedp.Poll[chromedp.Void](`document.querySelectorAll("[data-photo-item] [name=photo_data_uri]").length === 1 && !document.querySelector("[data-photo-submit]").disabled`),
		chromedp.Click(css(`[data-photo-submit]`)),
		chromedp.Poll[chromedp.Void](`location.pathname === "/dines"`),

		// The log form, also reached through an hx-boost navigation.
		chromedp.Click(css(`a[href="/log"]`)),
		chromedp.WaitVisible(css(`[data-log-form]`)),
		chromedp.Evaluate[chromedp.Void](helpers),
		chromedp.SetUploadFiles(css(`[data-photo-input]`), []string{photoPath}),
		// A valid log form, so only the pending photo stands in the way.
		chromedp.Evaluate[chromedp.Void](`for (const [name, value] of [["restaurant_name", "Slow Photo Diner"], ["rating_`+people[0].ID.String()+`", "7"]]) { const field = document.querySelector("[name=\"" + name + "\"]"); field.value = value; field.dispatchEvent(new Event("input", { bubbles: true })); }`),
		chromedp.Poll[chromedp.Void](`document.querySelector("[data-log-submit]").disabled`),
		evalInto(attempt, &sentLog),
		evalInto(`document.querySelector("[data-photo-error]").textContent`, &logError),
	)
	if err := chromedp.Do(browserCtx, actions...); err != nil {
		t.Fatal(err)
	}
	if !boosted {
		t.Fatal("edit page was reached by a full page load; the test must cover hx-boost navigation")
	}
	if sentEdit != 0 || editDisabled != "true" || editError != photoPendingMessage {
		t.Fatalf("edit form while compressing: %d requests sent, disabled=%s, error=%q", sentEdit, editDisabled, editError)
	}
	if sentLog != 0 || logError != photoPendingMessage {
		t.Fatalf("log form while compressing: %d requests sent, error=%q", sentLog, logError)
	}

	visit, err := store.Visit(ctx, visitID)
	if err != nil {
		t.Fatal(err)
	}
	if len(visit.Photos) != 1 {
		t.Fatalf("saved dine has %d photos, want the one picked during compression", len(visit.Photos))
	}
}

// A canvas that cannot be drawn on (getContext returns null) must fail the
// photo, not leave it pending and the save button disabled for good.
func TestPhotoCompressionFailureReleasesPendingPhoto(t *testing.T) {
	chromePath := chromeExecutableForTest()
	if chromePath == "" {
		t.Skip("Chrome or Chromium executable not found")
	}
	t.Chdir(filepath.Join("..", ".."))

	store := repository.NewMemoryStore()
	_, visitID := seedSlowPhotoVisit(t, store)
	photoPath := writePickPhoto(t)
	router, token := newAuthenticatedTestRouter(t, store)
	server := httptest.NewServer(router)
	defer server.Close()
	browserCtx, cancelBrowser := newPhotoBrowserContext(t, chromePath)
	defer cancelBrowser()

	var disabled, errorText string
	var tiles int
	actions := append(openEditPageByBoost(server.URL, token, visitID),
		chromedp.Evaluate[chromedp.Void](`HTMLCanvasElement.prototype.getContext = () => null`),
		chromedp.SetUploadFiles(css(`[data-photo-input]`), []string{photoPath}),
		chromedp.Poll[chromedp.Void](`!document.querySelector("[data-photo-error]").hidden`),
		evalInto(`document.querySelector("[data-photo-error]").textContent`, &errorText),
		evalInto(`String(document.querySelector("[data-photo-submit]").disabled)`, &disabled),
		evalInto(`document.querySelectorAll("[data-photo-item]").length`, &tiles),
	)
	if err := chromedp.Do(browserCtx, actions...); err != nil {
		t.Fatal(err)
	}
	if errorText == "" || errorText == photoPendingMessage {
		t.Fatalf("photo error = %q, want the compression failure", errorText)
	}
	if disabled != "false" || tiles != 0 {
		t.Fatalf("after failed compression: save disabled=%s, %d photo tiles; want enabled and none", disabled, tiles)
	}
}

// An edit form cached by htmx while a photo compressed has its save button
// disabled; restoring it on Back must not leave the button stuck.
func TestEditFormRestoredFromHistoryEnablesSave(t *testing.T) {
	chromePath := chromeExecutableForTest()
	if chromePath == "" {
		t.Skip("Chrome or Chromium executable not found")
	}
	t.Chdir(filepath.Join("..", ".."))

	store := repository.NewMemoryStore()
	_, visitID := seedSlowPhotoVisit(t, store)
	photoPath := writePickPhoto(t)
	router, token := newAuthenticatedTestRouter(t, store)
	server := httptest.NewServer(router)
	defer server.Close()
	browserCtx, cancelBrowser := newPhotoBrowserContext(t, chromePath)
	defer cancelBrowser()

	// The picked photo stays "compressing" because its image never loads.
	const holdImages = `(() => {
		const RealImage = window.Image;
		window.Image = function () {
			const img = new RealImage();
			Object.defineProperty(img, "src", { configurable: true, set() {} });
			return img;
		};
	})()`
	var boosted bool
	var disabledBefore, disabledAfter string
	actions := append(openEditPageByBoost(server.URL, token, visitID),
		chromedp.Evaluate[chromedp.Void](holdImages),
		chromedp.SetUploadFiles(css(`[data-photo-input]`), []string{photoPath}),
		chromedp.Poll[chromedp.Void](`document.querySelector("[data-photo-submit]").disabled`),
		evalInto(`String(document.querySelector("[data-photo-submit]").disabled)`, &disabledBefore),
		// Leave through an hx-boost link so htmx caches the disabled form.
		chromedp.Click(css(`a[href="/log"]`)),
		chromedp.WaitVisible(css(`[data-log-form]`)),
		chromedp.NavigateBack(),
		chromedp.WaitVisible(css(`[data-photo-submit]`)),
		evalInto(`window.__boostMarker === true`, &boosted),
		evalInto(`String(document.querySelector("[data-photo-submit]").disabled)`, &disabledAfter),
	)
	if err := chromedp.Do(browserCtx, actions...); err != nil {
		t.Fatal(err)
	}
	if disabledBefore != "true" {
		t.Fatalf("save disabled while compressing = %s, want true", disabledBefore)
	}
	if !boosted {
		t.Fatal("Back reloaded the page; the test must cover the htmx history restore")
	}
	if disabledAfter != "false" {
		t.Fatalf("restored edit form has save disabled=%s, want false", disabledAfter)
	}
}

// seedSlowPhotoVisit stores one dine to edit and returns the people and its ID.
func seedSlowPhotoVisit(t *testing.T, store *repository.MemoryStore) ([]model.Person, uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Slow Photo Diner",
		VisitedAt:      time.Now(),
		PickerID:       &people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	return people, *visitID
}

func writePickPhoto(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pick.jpg")
	if err := os.WriteFile(path, testutil.JPEG(64, 48, 3), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func newPhotoBrowserContext(t *testing.T, chromePath string) (context.Context, context.CancelFunc) {
	t.Helper()
	options := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(chromePath),
		chromedp.Headless,
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.DisableGPU,
		chromedp.Flag("disable-dev-shm-usage", true),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), options...)
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx, chromedp.WithLogf(func(string, ...any) {}))
	browserCtx, cancelTimeout := context.WithTimeout(browserCtx, 30*time.Second)
	return browserCtx, func() {
		cancelTimeout()
		cancelBrowser()
		cancelAlloc()
	}
}

// openEditPageByBoost loads /dines, then follows the dine's edit link with
// hx-boost and waits for the photo form.
func openEditPageByBoost(serverURL, token string, visitID uuid.UUID) []chromedp.Action[chromedp.Void] {
	link := css(`a[href^="/visits/` + visitID.String() + `/edit"]`)
	return []chromedp.Action[chromedp.Void]{
		enableNetwork(),
		chromedp.Navigate(serverURL + "/health"),
		setSessionCookie(serverURL, token),
		chromedp.Navigate(serverURL + "/dines"),
		chromedp.WaitVisible(link),
		chromedp.Evaluate[chromedp.Void](`window.__boostMarker = true`),
		chromedp.Click(link),
		chromedp.WaitVisible(css(`[data-photo-submit]`)),
	}
}
