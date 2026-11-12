package server

import (
	"context"
	"fmt"
	"math"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/bitofbytes-io/dined/internal/testutil"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
	"github.com/google/uuid"
)

func TestHomeCTAFitsBothAudiencesAtMobileAndDesktopWidths(t *testing.T) {
	chrome := chromeExecutableForTest()
	if chrome == "" {
		t.Skip("Chrome or Chromium executable not found")
	}
	t.Chdir(filepath.Join("..", ".."))
	// Use the synthetic memory fixture and real session middleware; no external
	// identity provider, personal photos, or production credentials are needed.
	router, token := newAuthenticatedTestRouter(t, repository.NewMemoryStore())
	server := httptest.NewServer(router)
	defer server.Close()
	opts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(chrome), chromedp.Headless, chromedp.NoFirstRun, chromedp.NoDefaultBrowserCheck, chromedp.DisableGPU)
	alloc, stopAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	defer stopAlloc()
	for _, audience := range []struct {
		name, href, alt string
		authenticated   bool
	}{
		{"logged-out", "/api/auth/google", "Log In", false},
		{"logged-in", "/log", "Log a Dine", true},
	} {
		t.Run(audience.name, func(t *testing.T) {
			browser, closeBrowser := chromedp.NewContext(alloc, chromedp.WithLogf(func(string, ...any) {}))
			defer closeBrowser()
			browser, cancel := context.WithTimeout(browser, 45*time.Second)
			defer cancel()
			if err := chromedp.Do(browser, enableNetwork(), clearBrowserCookies(), chromedp.Navigate(server.URL+"/health")); err != nil {
				t.Fatal(err)
			}
			if audience.authenticated {
				if err := chromedp.Do(browser, setSessionCookie(server.URL, token)); err != nil {
					t.Fatal(err)
				}
			}
			for _, width := range []int64{360, 390, 414, 1440} {
				t.Run(fmt.Sprint(width), func(t *testing.T) {
					height := int64(844)
					if width == 1440 {
						height = 900
					}
					if err := chromedp.Do(browser,
						setEmulatedMediaFeatures([]*emulation.MediaFeature{{Name: "prefers-reduced-motion", Value: "no-preference"}}),
						chromedp.EmulateViewport(width, height), chromedp.Navigate(server.URL+"/"),
						chromedp.Poll[chromedp.Void](`(()=>{const i=document.querySelector('.image-cta img');return i && i.complete && i.naturalWidth>0;})()`),
						chromedp.Poll[chromedp.Void](`document.getAnimations().every(a=>a.playState!=="running" || a.effect.getComputedTiming().iterations===Infinity)`),
					); err != nil {
						t.Fatal(err)
					}
					if dir := os.Getenv("DINED_HOME_SCREENSHOT_DIR"); dir != "" {
						if err := os.MkdirAll(dir, 0700); err != nil {
							t.Fatal(err)
						}
						data, err := chromedp.Run(browser, chromedp.CaptureScreenshot())
						if err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%s-%d.png", audience.name, width)), data, 0600); err != nil {
							t.Fatal(err)
						}
					}
					var metrics struct {
						SearchWidth float64 `json:"searchWidth"`
						Href        string  `json:"href"`
						Alt         string  `json:"alt"`
						Width       float64 `json:"width"`
						Height      float64 `json:"height"`
						Left        float64 `json:"left"`
						Right       float64 `json:"right"`
						Visible     bool    `json:"visible"`
						Loaded      bool    `json:"loaded"`
						Overflow    bool    `json:"overflow"`
						Animated    bool    `json:"animated"`
					}
					if err := chromedp.Do(browser, evalInto(`(()=>{
      const a=document.querySelector('.image-cta'), i=a.querySelector('img'), r=a.getBoundingClientRect(), s=getComputedStyle(a);
      return {searchWidth:document.querySelector('.booth-search').getBoundingClientRect().width,href:a.getAttribute('href'),alt:i.alt,width:r.width,height:r.height,left:r.left,right:r.right,
       visible:s.display!=='none' && s.visibility==='visible' && r.width>0 && r.height>0,
       loaded:i.complete && i.naturalWidth>0,overflow:document.documentElement.scrollWidth>innerWidth,
       animated:s.animationName==='arrowFloat' && s.animationIterationCount==='infinite'};
     })()`, &metrics)); err != nil {
						t.Fatal(err)
					}
					if metrics.Href != audience.href || metrics.Alt != audience.alt {
						t.Errorf("CTA destination/label = %q/%q, want %q/%q", metrics.Href, metrics.Alt, audience.href, audience.alt)
					}
					if !metrics.Visible || !metrics.Loaded {
						t.Fatalf("CTA visible=%v, loaded=%v", metrics.Visible, metrics.Loaded)
					}
					if metrics.Width > 224 || metrics.Height < 44 || metrics.Left < 0 || metrics.Right > float64(width) || metrics.Overflow {
						t.Errorf("CTA does not fit viewport: %+v", metrics)
					}
					if width <= 414 && math.Abs((metrics.Left+metrics.Right)/2-float64(width)/2) > 4 {
						t.Error("mobile CTA is not centered")
					}
					if width == 1440 && metrics.SearchWidth < 720 {
						t.Error("CTA intrinsic size squeezed the desktop search panel")
					}
					if !metrics.Animated {
						t.Error("CTA lost its floating animation")
					}
					// Reach the link with actual keyboard navigation, including on phones
					// where the collapsed menu changes the tab sequence.
					focused := false
					for n := 0; n < 30 && !focused; n++ {
						if err := chromedp.Do(browser, chromedp.KeyEvent(kb.Tab), evalInto(`document.activeElement===document.querySelector('.image-cta')`, &focused)); err != nil {
							t.Fatal(err)
						}
					}
					var focusVisible, reduced bool
					if err := chromedp.Do(browser, evalInto(`document.querySelector('.image-cta').matches(':focus-visible') && getComputedStyle(document.querySelector('.image-cta')).outlineStyle!=='none'`, &focusVisible),
						setEmulatedMediaFeatures([]*emulation.MediaFeature{{Name: "prefers-reduced-motion", Value: "reduce"}}),
						evalInto(`(()=>{const s=getComputedStyle(document.querySelector('.image-cta'));return parseFloat(s.animationDuration)<0.01 && s.animationIterationCount==='1';})()`, &reduced),
					); err != nil {
						t.Fatal(err)
					}
					if !focused || !focusVisible {
						t.Error("CTA cannot receive visible keyboard focus")
					}
					if !reduced {
						t.Error("CTA ignores reduced-motion preference")
					}
				})
			}
		})
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
				{DataURI: testutil.PhotoURI(1)},
				{DataURI: testutil.PhotoURI(2)},
				{DataURI: testutil.PhotoURI(9)},
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
	err = chromedp.Do(browserCtx,
		enableNetwork(),
		chromedp.EmulateViewport(1440, 900),
		chromedp.Navigate(server.URL+"/health"),
		setSessionCookie(server.URL, token),
		chromedp.Navigate(server.URL+"/"),
		chromedp.WaitVisible(css(`.booth-recent .visit-card`)),
		evalInto(`(() => {
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
