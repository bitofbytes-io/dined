package handler

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/ui"
)

const (
	// mapsLinkParam carries a pasted Google Maps link to the save form page.
	mapsLinkParam    = "maps_url"
	maxMapsLinkBytes = 2048

	notMapsLinkMessage         = "That doesn't look like a Google Maps link. Paste a maps.app.goo.gl or google.com/maps/place link."
	unresolvedMapsLinkMessage  = "Couldn't find a place for that link. Check it opens in Google Maps, or search for the place by name."
	mapsLinkLookupFailMessage  = "Could not look up that link. Please try again."
	mapsLinkNotConfiguredError = "Google Places is not configured for this environment, so links cannot be looked up."
)

// wantToTryLinkValues resolves a pasted Google Maps link into the save form
// query for its place. On failure it shows the Want to Try list again with the
// reason and the pasted link, and returns false.
func (h *Handler) wantToTryLinkValues(w http.ResponseWriter, r *http.Request, link string) (url.Values, bool) {
	fail := func(status int, message string) (url.Values, bool) {
		h.renderWantToTry(w, r, status, ui.PageData{Error: message, MapsLink: link})
		return nil, false
	}
	if len(link) > maxMapsLinkBytes || !places.IsMapsLink(link) {
		return fail(http.StatusBadRequest, notMapsLinkMessage)
	}
	if !h.placesConfigured() {
		return fail(http.StatusServiceUnavailable, mapsLinkNotConfiguredError)
	}
	place, err := h.places.ResolveMapsLink(r.Context(), link)
	switch {
	case errors.Is(err, places.ErrNotMapsLink):
		return fail(http.StatusBadRequest, notMapsLinkMessage)
	case errors.Is(err, places.ErrLinkUnresolved):
		slog.Warn("want to try maps link unresolved", "error", err)
		return fail(http.StatusUnprocessableEntity, unresolvedMapsLinkMessage)
	case err != nil:
		slog.Error("want to try maps link", "error", err)
		return fail(http.StatusInternalServerError, mapsLinkLookupFailMessage)
	}
	if place.DisplayName.Text == "" {
		return fail(http.StatusUnprocessableEntity, unresolvedMapsLinkMessage)
	}
	values := ui.WantToTryPlaceValues(*place)
	values.Set("return_to", r.URL.Query().Get("return_to"))
	return values, true
}
