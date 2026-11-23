package places

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	// ErrNotMapsLink means the text is not a Google Maps link, so nothing was requested for it.
	ErrNotMapsLink = errors.New("not a Google Maps link")
	// ErrLinkUnresolved means the link is a Google Maps link but did not lead to a place.
	ErrLinkUnresolved = errors.New("google maps link did not resolve to a place")
)

const (
	mapsLinkMaxRedirects = 5
	mapsLinkHopTimeout   = 4 * time.Second
	mapsLinkTimeout      = 12 * time.Second
	// mapsLinkMaxMiles is how far from the link's coordinates a found place may be.
	mapsLinkMaxMiles = 0.5
	// mapsLinkPinMiles is how close a result must be to the link's pin to be
	// taken over other results in range, and mapsLinkMinGapMiles is how much
	// farther the next one must be. Anything less is ambiguous.
	mapsLinkPinMiles    = 0.05
	mapsLinkMinGapMiles = 0.05
	placeIDPrefix       = "place_id:"
)

var (
	placeIDPattern     = regexp.MustCompile(`^[A-Za-z0-9_-]{10,200}$`)
	dataCoordsPattern  = regexp.MustCompile(`!3d(-?\d+(?:\.\d+)?)!4d(-?\d+(?:\.\d+)?)`)
	viewCoordsPattern  = regexp.MustCompile(`@(-?\d+(?:\.\d+)?),(-?\d+(?:\.\d+)?)`)
	plainCoordsPattern = regexp.MustCompile(`^\s*(-?\d+(?:\.\d+)?)\s*,\s*(-?\d+(?:\.\d+)?)\s*$`)
)

// mapsLinkKind says how a Google host is allowed to be used.
type mapsLinkKind int

const (
	notAllowed mapsLinkKind = iota
	// shortLink hosts answer with a redirect that has to be followed.
	shortLink
	// pageLink hosts carry the place in the URL itself and are never requested.
	pageLink
)

// classifyMapsURL accepts only https links on Google's Maps hosts. It is the
// one allowlist: every URL the resolver requests or follows passes through it.
func classifyMapsURL(u *url.URL) mapsLinkKind {
	if u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return notAllowed
	}
	switch strings.ToLower(u.Hostname()) {
	case "maps.app.goo.gl":
		return shortLink
	case "goo.gl":
		if strings.HasPrefix(u.Path, "/maps") {
			return shortLink
		}
	case "google.com", "www.google.com":
		if u.Path == "/maps" || strings.HasPrefix(u.Path, "/maps/") {
			return pageLink
		}
	case "maps.google.com":
		return pageLink
	}
	return notAllowed
}

// IsMapsLink reports whether raw is an https link on one of Google's Maps
// hosts. It makes no request.
func IsMapsLink(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && classifyMapsURL(u) != notAllowed
}

// ResolveMapsLink turns a pasted Google Maps share link into a place. Short
// links are followed by hand, never to a host outside the allowlist, and the
// place is then read from the final URL: its place ID when it has one, else
// its name searched through Places near its coordinates.
func (c *Client) ResolveMapsLink(ctx context.Context, raw string) (*Place, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("google places client is not configured")
	}
	start, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || classifyMapsURL(start) == notAllowed {
		return nil, ErrNotMapsLink
	}
	ctx, cancel := context.WithTimeout(ctx, mapsLinkTimeout)
	defer cancel()
	final, err := c.followMapsLink(ctx, start)
	if err != nil {
		return nil, err
	}
	ref := parseMapsReference(final)
	switch {
	case ref.placeID != "":
		place, err := c.Details(ctx, ref.placeID)
		if err != nil {
			return nil, fmt.Errorf("google places details for maps link: %w", err)
		}
		if place == nil || place.ID == "" {
			return nil, ErrLinkUnresolved
		}
		return place, nil
	case ref.name != "":
		return c.placeByName(ctx, ref)
	}
	return nil, ErrLinkUnresolved
}

// followMapsLink follows short-link redirects one hop at a time until it
// reaches a page link, checking each hop against the allowlist before any
// request is made.
func (c *Client) followMapsLink(ctx context.Context, current *url.URL) (*url.URL, error) {
	httpClient := &http.Client{
		Timeout:       mapsLinkHopTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	if c.http != nil {
		httpClient.Transport = c.http.Transport
	}
	seen := make(map[string]bool)
	for hop := 0; ; hop++ {
		switch classifyMapsURL(current) {
		case notAllowed:
			return nil, ErrNotMapsLink
		case pageLink:
			return current, nil
		}
		key := current.String()
		if seen[key] {
			return nil, fmt.Errorf("%w: redirect loop", ErrLinkUnresolved)
		}
		if hop >= mapsLinkMaxRedirects {
			return nil, fmt.Errorf("%w: too many redirects", ErrLinkUnresolved)
		}
		seen[key] = true
		next, err := redirectTarget(ctx, httpClient, current)
		if err != nil {
			return nil, err
		}
		current = next
	}
}

func redirectTarget(ctx context.Context, httpClient *http.Client, current *url.URL) (*url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, current.String(), nil)
	if err != nil {
		return nil, ErrLinkUnresolved
	}
	res, err := httpClient.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("google maps link request failed: %w", ctxErr)
		}
		return nil, fmt.Errorf("google maps link request failed")
	}
	res.Body.Close()
	if res.StatusCode < 300 || res.StatusCode > 399 {
		return nil, fmt.Errorf("%w: status %d", ErrLinkUnresolved, res.StatusCode)
	}
	location, err := res.Location()
	if err != nil {
		return nil, fmt.Errorf("%w: no redirect target", ErrLinkUnresolved)
	}
	return location, nil
}

// mapsReference is what a resolved Maps URL says about its place.
type mapsReference struct {
	placeID   string
	name      string
	latitude  float64
	longitude float64
	hasCoords bool
}

func parseMapsReference(u *url.URL) mapsReference {
	var ref mapsReference
	query := u.Query()
	q := query.Get("q")
	if q == "" {
		q = query.Get("query")
	}
	if id := query.Get("query_place_id"); placeIDPattern.MatchString(id) {
		ref.placeID = id
	} else if id := strings.TrimPrefix(q, placeIDPrefix); id != q && placeIDPattern.MatchString(id) {
		ref.placeID = id
	}
	if segments := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/"); len(segments) >= 3 && segments[0] == "maps" && segments[1] == "place" {
		// "@lat,lng,zoom" in the name slot is a map view, not a place.
		if name, err := url.QueryUnescape(segments[2]); err == nil && !plainCoordsPattern.MatchString(name) && !strings.HasPrefix(name, "@") {
			ref.name = strings.TrimSpace(name)
		}
	}
	ref.latitude, ref.longitude, ref.hasCoords = mapsCoordinates(u)
	return ref
}

// mapsCoordinates prefers the place's own pin (!3d..!4d..) over the map view
// center (@lat,lng).
func mapsCoordinates(u *url.URL) (float64, float64, bool) {
	for _, pattern := range []*regexp.Regexp{dataCoordsPattern, viewCoordsPattern} {
		if m := pattern.FindStringSubmatch(u.EscapedPath()); m != nil {
			if lat, lng, ok := validCoordinates(m[1], m[2]); ok {
				return lat, lng, true
			}
		}
	}
	if m := plainCoordsPattern.FindStringSubmatch(u.Query().Get("q")); m != nil {
		return validCoordinates(m[1], m[2])
	}
	return 0, 0, false
}

func validCoordinates(latText, lngText string) (float64, float64, bool) {
	lat, latErr := strconv.ParseFloat(latText, 64)
	lng, lngErr := strconv.ParseFloat(lngText, 64)
	if latErr != nil || lngErr != nil || math.Abs(lat) > 90 || math.Abs(lng) > 180 {
		return 0, 0, false
	}
	return lat, lng, true
}

// placeByName searches Places for the link's name near its coordinates and
// takes the match that sits on them. A name alone is not enough: for a chain it
// would pick an arbitrary branch, so a link without coordinates is left
// unresolved. When several results are near, the one on the pin wins only if
// it is clearly closer than the rest; otherwise the link is ambiguous. Whatever
// is chosen must also carry the link's name, so a stale link whose place has
// closed or moved never prefills a different restaurant.
func (c *Client) placeByName(ctx context.Context, ref mapsReference) (*Place, error) {
	if !ref.hasCoords {
		return nil, ErrLinkUnresolved
	}
	found, err := c.TextSearchNear(ctx, ref.name, ref.latitude, ref.longitude)
	if err != nil {
		return nil, fmt.Errorf("google places search for maps link: %w", err)
	}
	var nearest *Place
	nearestMiles, runnerUpMiles := math.Inf(1), math.Inf(1)
	count := 0
	for i := range found {
		miles := DistanceMiles(ref.latitude, ref.longitude, found[i])
		if found[i].ID == "" || miles > mapsLinkMaxMiles {
			continue
		}
		count++
		if miles < nearestMiles {
			nearest, runnerUpMiles, nearestMiles = &found[i], nearestMiles, miles
		} else if miles < runnerUpMiles {
			runnerUpMiles = miles
		}
	}
	switch {
	case count == 1, count > 1 && nearestMiles <= mapsLinkPinMiles && runnerUpMiles-nearestMiles >= mapsLinkMinGapMiles:
		if namesMatch(ref.name, nearest.DisplayName.Text) {
			return nearest, nil
		}
	}
	return nil, ErrLinkUnresolved
}

// namesMatch reports whether two place names are the same once case and
// punctuation are ignored, or one is the other with words added or dropped
// ("Hank's" and "Hank's Downtown Diner").
func namesMatch(linked, found string) bool {
	a, b := nameWords(linked), nameWords(found)
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	for start := 0; start+len(a) <= len(b); start++ {
		if slices.Equal(a, b[start:start+len(a)]) {
			return true
		}
	}
	return false
}

// nameWords lowercases name and splits it into runs of letters and digits,
// dropping apostrophes so "Hank's" and "Hanks" agree.
func nameWords(name string) []string {
	name = strings.NewReplacer("'", "", "’", "").Replace(strings.ToLower(name))
	return strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}
