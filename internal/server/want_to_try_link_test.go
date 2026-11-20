package server

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/bitofbytes-io/dined/internal/repository"
)

func TestRouterWantToTryListOffersMapsLinkForm(t *testing.T) {
	router, token := newAuthenticatedTestRouter(t, repository.NewMemoryStore())

	body := getWithSession(router, token, "/want-to-try").Body.String()

	for _, want := range []string{`action="/want-to-try/new"`, `name="maps_url"`, "Add from a Google Maps link"} {
		if !strings.Contains(body, want) {
			t.Fatalf("list missing %q:\n%s", want, body)
		}
	}
}

func TestRouterWantToTryMapsLinkErrorKeepsThePastedLink(t *testing.T) {
	router, token := newAuthenticatedTestRouter(t, repository.NewMemoryStore())
	link := "https://maps.app.goo.gl/abc123"

	rec := getWithSession(router, token, "/want-to-try/new?"+url.Values{"maps_url": {link}}.Encode())

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
