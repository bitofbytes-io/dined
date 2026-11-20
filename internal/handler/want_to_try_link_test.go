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
	target := "/want-to-try/new?" + url.Values{"maps_url": {link}}.Encode()
	return httptest.NewRequestWithContext(context.Background(), http.MethodGet, target, nil)
}

func TestNewWantToTryPagePrefillsFromMapsLink(t *testing.T) {
	fake := &fakeGooglePlacesClient{configured: true, linkPlace: &places.Place{
		ID:          "ChIJhanks",
		DisplayName: places.Name{Text: "Hank's Downtown Diner"},
		Address:     "1 Main St, Raleigh, NC",
		Location:    places.Location{Latitude: 35.7797, Longitude: -78.6383},
		AddressComponents: []places.AddressComponent{
			{LongText: "Raleigh", Types: []string{"locality"}},
		},
	}}
	rec := httptest.NewRecorder()

	New(nil, repository.NewMemoryStore(), fake, nil, nil).NewWantToTryPage(rec, newWantToTryLinkRequest("https://maps.app.goo.gl/abc"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
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

func TestNewWantToTryPageShowsErrorForBadMapsLink(t *testing.T) {
	tests := []struct {
		name       string
		link       string
		configured bool
		err        error
		place      *places.Place
		status     int
		message    string
	}{
		{"not a maps link", "https://example.com/maps/place/Hanks", true, places.ErrNotMapsLink, nil, http.StatusBadRequest, notMapsLinkMessage},
		{"unresolvable", "https://maps.app.goo.gl/abc", true, places.ErrLinkUnresolved, nil, http.StatusUnprocessableEntity, unresolvedMapsLinkMessage},
		{"redirect loop", "https://maps.app.goo.gl/loop", true, errors.Join(places.ErrLinkUnresolved, errors.New("redirect loop")), nil, http.StatusUnprocessableEntity, unresolvedMapsLinkMessage},
		{"place without a name", "https://maps.app.goo.gl/abc", true, nil, &places.Place{ID: "x"}, http.StatusUnprocessableEntity, unresolvedMapsLinkMessage},
		{"lookup failure", "https://maps.app.goo.gl/abc", true, errors.New("boom"), nil, http.StatusInternalServerError, mapsLinkLookupFailMessage},
		{"places not configured", "https://maps.app.goo.gl/abc", false, nil, nil, http.StatusServiceUnavailable, mapsLinkNotConfiguredError},
		{"too long", "https://maps.app.goo.gl/" + strings.Repeat("a", maxMapsLinkBytes), true, nil, nil, http.StatusBadRequest, notMapsLinkMessage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeGooglePlacesClient{configured: tt.configured, linkErr: tt.err, linkPlace: tt.place}
			rec := httptest.NewRecorder()

			New(nil, repository.NewMemoryStore(), fake, nil, nil).NewWantToTryPage(rec, newWantToTryLinkRequest(tt.link))

			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
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
