package server

import (
	"context"
	"fmt"
	"math"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"
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
			if err := chromedp.Run(browser, network.Enable(), network.ClearBrowserCookies(), chromedp.Navigate(server.URL+"/health")); err != nil {
				t.Fatal(err)
			}
			if audience.authenticated {
				if err := chromedp.Run(browser, chromedp.ActionFunc(func(ctx context.Context) error {
					return network.SetCookie(middleware.CookieName, token).WithURL(server.URL).WithPath("/").Do(ctx)
				})); err != nil {
					t.Fatal(err)
				}
			}
			for _, width := range []int64{360, 390, 414, 1440} {
				t.Run(fmt.Sprint(width), func(t *testing.T) {
					height := int64(844)
					if width == 1440 {
						height = 900
					}
					if err := chromedp.Run(browser,
						emulation.SetEmulatedMedia().WithFeatures([]*emulation.MediaFeature{{Name: "prefers-reduced-motion", Value: "no-preference"}}),
						chromedp.EmulateViewport(width, height), chromedp.Navigate(server.URL+"/"),
						chromedp.Poll(`(()=>{const i=document.querySelector('.image-cta img');return i && i.complete && i.naturalWidth>0;})()`, nil),
						chromedp.Poll(`document.getAnimations().every(a=>a.playState!=="running" || a.effect.getComputedTiming().iterations===Infinity)`, nil),
					); err != nil {
						t.Fatal(err)
					}
					if dir := os.Getenv("DINED_HOME_SCREENSHOT_DIR"); dir != "" {
						if err := os.MkdirAll(dir, 0700); err != nil {
							t.Fatal(err)
						}
						var data []byte
						if err := chromedp.Run(browser, chromedp.CaptureScreenshot(&data)); err != nil {
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
					if err := chromedp.Run(browser, chromedp.Evaluate(`(()=>{
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
						if err := chromedp.Run(browser, chromedp.KeyEvent(kb.Tab), chromedp.Evaluate(`document.activeElement===document.querySelector('.image-cta')`, &focused)); err != nil {
							t.Fatal(err)
						}
					}
					var focusVisible, reduced bool
					if err := chromedp.Run(browser, chromedp.Evaluate(`document.querySelector('.image-cta').matches(':focus-visible') && getComputedStyle(document.querySelector('.image-cta')).outlineStyle!=='none'`, &focusVisible),
						emulation.SetEmulatedMedia().WithFeatures([]*emulation.MediaFeature{{Name: "prefers-reduced-motion", Value: "reduce"}}),
						chromedp.Evaluate(`(()=>{const s=getComputedStyle(document.querySelector('.image-cta'));return parseFloat(s.animationDuration)<0.01 && s.animationIterationCount==='1';})()`, &reduced),
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
