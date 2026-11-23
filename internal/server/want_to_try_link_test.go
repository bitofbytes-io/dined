package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/repository"
)

func TestRouterWantToTryListOffersMapsLinkForm(t *testing.T) {
	router, token := newAuthenticatedTestRouter(t, repository.NewMemoryStore())

	body := getWithSession(router, token, "/want-to-try").Body.String()

	for _, want := range []string{`method="post" action="/want-to-try/link"`, `name="maps_url"`, "Add from a Google Maps link"} {
		if !strings.Contains(body, want) {
			t.Fatalf("list missing %q:\n%s", want, body)
		}
	}
}

func postLinkWithSession(router http.Handler, token, link string, sameOrigin bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/want-to-try/link", strings.NewReader(url.Values{"maps_url": {link}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	if sameOrigin {
		setSameOrigin(req)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestRouterWantToTryMapsLinkLookupRejectsCrossSitePost(t *testing.T) {
	router, token := newAuthenticatedTestRouter(t, repository.NewMemoryStore())

	rec := postLinkWithSession(router, token, "https://maps.app.goo.gl/abc123", false)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRouterWantToTryMapsLinkLookupRejectsForeignOrigin(t *testing.T) {
	router, token := newAuthenticatedTestRouter(t, repository.NewMemoryStore())
	req := httptest.NewRequest(http.MethodPost, "/want-to-try/link", strings.NewReader(url.Values{"maps_url": {"https://maps.app.goo.gl/abc123"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "https://evil.example")
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRouterWantToTryMapsLinkErrorKeepsThePastedLink(t *testing.T) {
	router, token := newAuthenticatedTestRouter(t, repository.NewMemoryStore())
	link := "https://maps.app.goo.gl/abc123"

	rec := postLinkWithSession(router, token, link, true)

	// The test router has no Places key, so the link cannot be looked up.
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want HTML", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Google Places is not configured") || !strings.Contains(body, `value="`+link+`"`) {
		t.Fatalf("want the error and the pasted link back in the form:\n%s", body)
	}
}
