package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/repository"
)

func newWantToTryLinkRequest(link string) *http.Request {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/want-to-try/link", strings.NewReader(url.Values{"maps_url": {link}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func TestNewWantToTryPageDoesNotLookUpALink(t *testing.T) {
	fake := &fakeGooglePlacesClient{configured: true, linkPlace: &places.Place{ID: "x", DisplayName: places.Name{Text: "Hank's"}}}
	target := "/want-to-try/new?" + url.Values{"maps_url": {"https://maps.app.goo.gl/abc"}}.Encode()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
	rec := httptest.NewRecorder()

	New(nil, repository.NewMemoryStore(), fake, nil, nil).NewWantToTryPage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: a link alone names no restaurant", rec.Code, http.StatusBadRequest)
	}
	if len(fake.linkRequests) != 0 {
		t.Fatalf("resolver calls = %v, want none on GET", fake.linkRequests)
	}
}

func TestLookupWantToTryLinkPrefillsFromMapsLink(t *testing.T) {
	fake := &fakeGooglePlacesClient{configured: true, linkPlace: &places.Place{
		ID:          "ChIJhanks",
		DisplayName: places.Name{Text: "Hank's Downtown Diner"},
		Address:     "1 Main St, Raleigh, NC",
		Location:    places.Location{Latitude: 35.7797, Longitude: -78.6383},
		AddressComponents: []places.AddressComponent{
			{LongText: "Raleigh", Types: []string{"locality"}},
		},
	}}
	store := repository.NewMemoryStore()
	rec := httptest.NewRecorder()

	New(nil, store, fake, nil, nil).LookupWantToTryLink(rec, newWantToTryLinkRequest("https://maps.app.goo.gl/abc"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if items, err := store.WantToTry(context.Background()); err != nil || len(items) != 0 {
		t.Fatalf("want to try list = %v, err = %v, want it empty until the user confirms", items, err)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"<h1>Hank&#39;s Downtown Diner</h1>",
		"1 Main St, Raleigh, NC",
		`name="google_place_id" value="ChIJhanks"`,
		`name="restaurant_name" value="Hank&#39;s Downtown Diner"`,
		`name="city" value="Raleigh"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("form missing %q:\n%s", want, body)
		}
	}
	if len(fake.linkRequests) != 1 || fake.linkRequests[0] != "https://maps.app.goo.gl/abc" {
		t.Fatalf("link requests = %v", fake.linkRequests)
	}
}

func TestLookupWantToTryLinkBlankMapsLinkStaysOnTheList(t *testing.T) {
	for _, link := range []string{"", "   ", "\t "} {
		fake := &fakeGooglePlacesClient{configured: true}
		rec := httptest.NewRecorder()

		New(nil, repository.NewMemoryStore(), fake, nil, nil).LookupWantToTryLink(rec, newWantToTryLinkRequest(link))

		body := rec.Body.String()
		if rec.Code != http.StatusBadRequest || !strings.Contains(body, strings.ReplaceAll(notMapsLinkMessage, "'", "&#39;")) || !strings.Contains(body, "Want to Try</h1>") {
			t.Fatalf("link %q: status = %d, want the list page with the link error:\n%s", link, rec.Code, body)
		}
		if len(fake.linkRequests) != 0 {
			t.Fatalf("resolver calls = %v, want none", fake.linkRequests)
		}
	}
}

func TestLookupWantToTryLinkShowsErrorForBadMapsLink(t *testing.T) {
	tests := []struct {
		name       string
		link       string
		configured bool
		err        error
		place      *places.Place
		status     int
		message    string
		calls      int
	}{
		{"not a maps link", "https://example.com/maps/place/Hanks", true, places.ErrNotMapsLink, nil, http.StatusBadRequest, notMapsLinkMessage, 0},
		{"unresolvable", "https://maps.app.goo.gl/abc", true, places.ErrLinkUnresolved, nil, http.StatusUnprocessableEntity, unresolvedMapsLinkMessage, 1},
		{"redirect loop", "https://maps.app.goo.gl/loop", true, errors.Join(places.ErrLinkUnresolved, errors.New("redirect loop")), nil, http.StatusUnprocessableEntity, unresolvedMapsLinkMessage, 1},
		{"place without a name", "https://maps.app.goo.gl/abc", true, nil, &places.Place{ID: "x"}, http.StatusUnprocessableEntity, unresolvedMapsLinkMessage, 1},
		{"lookup failure", "https://maps.app.goo.gl/abc", true, errors.New("boom"), nil, http.StatusInternalServerError, mapsLinkLookupFailMessage, 1},
		{"places not configured", "https://maps.app.goo.gl/abc", false, nil, nil, http.StatusServiceUnavailable, mapsLinkNotConfiguredError, 0},
		{"too long", "https://maps.app.goo.gl/" + strings.Repeat("a", maxMapsLinkBytes), true, nil, nil, http.StatusBadRequest, notMapsLinkMessage, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeGooglePlacesClient{configured: tt.configured, linkErr: tt.err, linkPlace: tt.place}
			rec := httptest.NewRecorder()

			New(nil, repository.NewMemoryStore(), fake, nil, nil).LookupWantToTryLink(rec, newWantToTryLinkRequest(tt.link))

			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
			if len(fake.linkRequests) != tt.calls {
				t.Fatalf("resolver calls = %v, want %d", fake.linkRequests, tt.calls)
			}
			body := rec.Body.String()
			if !strings.Contains(body, strings.ReplaceAll(tt.message, "'", "&#39;")) {
				t.Fatalf("body missing message %q:\n%s", tt.message, body)
			}
			if !strings.Contains(body, "Want to Try</h1>") || strings.Contains(body, `name="google_place_id"`) {
				t.Fatalf("want the list page, not the save form:\n%s", body)
			}
		})
	}
}
