package server

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
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

func TestPhotoThumbnailsAndOriginalModalAtMobileWidth(t *testing.T) {
	chrome := chromeExecutableForTest()
	if chrome == "" {
		t.Skip("Chrome or Chromium executable not found")
	}
	t.Chdir(filepath.Join("..", ".."))
	store := repository.NewMemoryStore()
	ctx := context.Background()
	people, _ := store.People(ctx)
	sizes := [][2]int{{1280, 960}, {600, 1200}, {320, 320}, {8, 6}}
	input := model.VisitInput{RestaurantName: "Photo fixture diner", VisitedAt: time.Now(), PriceLevel: 2, Ratings: map[uuid.UUID]float64{people[0].ID: 9}}
	for i, size := range sizes {
		input.Photos = append(input.Photos, model.VisitPhotoInput{DataURI: testutil.URI(testutil.JPEG(size[0], size[1], byte(i+1)))})
	}
	visitID, err := store.CreateVisit(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	visit, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	router, token := newAuthenticatedTestRouter(t, store)
	srv := httptest.NewServer(router)
	defer srv.Close()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(chrome), chromedp.Headless, chromedp.NoFirstRun, chromedp.NoDefaultBrowserCheck, chromedp.DisableGPU)
	alloc, cancel := chromedp.NewExecAllocator(ctx, opts...)
	defer cancel()
	browser, closeBrowser := chromedp.NewContext(alloc, chromedp.WithLogf(func(string, ...any) {}))
	defer closeBrowser()
	browser, timeout := context.WithTimeout(browser, 45*time.Second)
	defer timeout()
	strip := fmt.Sprintf(`[data-photo-src="/photos/%s"]`, visit.Photos[0].ID)
	if err = chromedp.Do(browser, enableNetwork(), chromedp.EmulateViewport(390, 844), chromedp.Navigate(srv.URL+"/health"), setSessionCookie(srv.URL, token), chromedp.Navigate(srv.URL+"/dines"), chromedp.WaitVisible(css(strip)), chromedp.ScrollIntoView(css(strip)), chromedp.Poll[chromedp.Void](`Array.from(document.querySelectorAll('.visit-photo-strip img')).every(i=>i.complete && i.naturalWidth>0 && i.naturalWidth<=320 && i.naturalHeight<=320 && i.src.includes('?variant=thumb-v1'))`)); err != nil {
		t.Fatal(err)
	}
	capture := func(name string) {
		t.Helper()
		if dir := os.Getenv("DINED_PHOTO_SCREENSHOT_DIR"); dir != "" {
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			if err := chromedp.Do(browser, chromedp.Poll[chromedp.Void](`document.getAnimations().every(a=>a.playState!=="running" || a.effect.getComputedTiming().iterations===Infinity)`)); err != nil {
				t.Fatal(err)
			}
			data, err := chromedp.Run(browser, chromedp.CaptureScreenshot())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	capture("thumbnail-strip.png")
	var manifest []map[string]any
	for i, photo := range visit.Photos {
		selector := fmt.Sprintf(`[data-photo-src="/photos/%s"]`, photo.ID)
		expected := fmt.Sprintf(`(()=>{let i=document.querySelector('#photo-preview-image');return i.complete && i.naturalWidth===%d && i.naturalHeight===%d && new URL(i.src).pathname==='/photos/%s' && new URL(i.src).search==='';})()`, sizes[i][0], sizes[i][1], photo.ID)
		if err = chromedp.Do(browser, chromedp.Click(css(selector)), chromedp.WaitVisible(css(`#photo-preview-modal[open]`)), chromedp.Poll[chromedp.Void](expected)); err != nil {
			t.Fatal(err)
		}
		if dir := os.Getenv("DINED_PHOTO_SCREENSHOT_DIR"); dir != "" {
			if err = os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			screenshot, err := chromedp.Run(browser, chromedp.CaptureScreenshot())
			if err != nil {
				t.Fatal(err)
			}
			file := fmt.Sprintf("photo-%02d-%s.png", i+1, photo.ID)
			if err = os.WriteFile(filepath.Join(dir, file), screenshot, 0600); err != nil {
				t.Fatal(err)
			}
			manifest = append(manifest, map[string]any{"id": photo.ID, "file": file, "width": sizes[i][0], "height": sizes[i][1], "sha256": fmt.Sprintf("%x", sha256.Sum256(testutil.JPEG(sizes[i][0], sizes[i][1], byte(i+1))))})
		}
		if i == len(visit.Photos)-1 {
			previous := fmt.Sprintf(`document.querySelector('#photo-preview-image').complete && new URL(document.querySelector('#photo-preview-image').src).pathname==='/photos/%s'`, visit.Photos[i-1].ID)
			if err = chromedp.Do(browser, chromedp.Click(css(`#photo-preview-prev`)), chromedp.Poll[chromedp.Void](previous), chromedp.Click(css(`#photo-preview-next`)), chromedp.Poll[chromedp.Void](expected)); err != nil {
				t.Fatal(err)
			}
		}
		if err = chromedp.Do(browser, chromedp.Click(css(`.photo-preview-close`)), chromedp.Poll[chromedp.Void](`!document.querySelector('#photo-preview-modal').open`)); err != nil {
			t.Fatal(err)
		}
	}
	if dir := os.Getenv("DINED_PHOTO_SCREENSHOT_DIR"); dir != "" {
		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Saved-photo editing uses the same bounded URLs, while preview buttons retain
	// original URLs. Existing form payloads carry only IDs for kept photos.
	if err = chromedp.Do(browser, chromedp.Navigate(srv.URL+"/visits/"+visitID.String()+"/edit"), chromedp.WaitVisible(css(`[data-photo-item]`)), chromedp.Poll[chromedp.Void](`document.querySelectorAll('[name=keep_photo_id]').length===4 && document.querySelectorAll('[name=photo_data_uri]').length===0 && Array.from(document.querySelectorAll('[data-photo-item] img')).every(i=>new URL(i.src).search==='?variant=thumb-v1')`)); err != nil {
		t.Fatal(err)
	}
	if err = chromedp.Do(browser, chromedp.ScrollIntoView(css(`[data-photo-item]`)), chromedp.Poll[chromedp.Void](`Array.from(document.querySelectorAll("[data-photo-item] img")).every(i=>i.complete && i.naturalWidth>0)`)); err != nil {
		t.Fatal(err)
	}
	capture("saved-photo-edit.png")
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
			{DataURI: testutil.PhotoURI(1)},
			{DataURI: testutil.PhotoURI(2)},
			{DataURI: testutil.PhotoURI(9)},
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
	err = chromedp.Do(browserCtx,
		enableNetwork(),
		chromedp.EmulateViewport(390, 844),
		chromedp.Navigate(server.URL+"/health"),
		setSessionCookie(server.URL, token),
		chromedp.Navigate(server.URL+"/visits/"+visitID.String()+"/edit"),
		chromedp.WaitVisible(css(`[data-photo-add]`)),
		evalInto(`(() => {
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
