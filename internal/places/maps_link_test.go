package places

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const (
	hanksPlaceURL = "https://www.google.com/maps/place/Hank's+Downtown+Diner/@35.7796,-78.6382,17z/data=!3m1!4b1!4m6!3m5!1s0x89ac:0x1!8m2!3d35.7797!4d-78.6383!16s%2Fg%2F1"
	hanksPlaceID  = "ChIJhanks0000000000"
)

// linkTransport answers by host and path, and records every request so tests
// can show which hosts were contacted.
type linkTransport struct {
	mu       sync.Mutex
	requests []string
	handle   func(req *http.Request) *http.Response
}

func (l *linkTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	l.mu.Lock()
	l.requests = append(l.requests, req.URL.String())
	l.mu.Unlock()
	res := l.handle(req)
	if res == nil {
		res = respond(http.StatusNotFound, "", nil)
	}
	res.Request = req
	return res, nil
}

func (l *linkTransport) hosts() map[string]bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	hosts := map[string]bool{}
	for _, raw := range l.requests {
		hosts[strings.SplitN(strings.TrimPrefix(raw, "https://"), "/", 2)[0]] = true
	}
	return hosts
}

func respond(status int, body string, header map[string]string) *http.Response {
	res := &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
	for key, value := range header {
		res.Header.Set(key, value)
	}
	return res
}

func redirect(to string) *http.Response {
	return respond(http.StatusFound, "", map[string]string{"Location": to})
}

func linkClient(transport *linkTransport) *Client {
	return &Client{apiKey: "test-key", http: &http.Client{Transport: transport}}
}

func TestResolveMapsLinkShortLinkFollowsRedirectAndSearchesByNameAndCoordinates(t *testing.T) {
	var searchBody string
	transport := &linkTransport{handle: func(req *http.Request) *http.Response {
		switch req.URL.Host {
		case "maps.app.goo.gl":
			return redirect(hanksPlaceURL)
		case "places.googleapis.com":
			data, _ := io.ReadAll(req.Body)
			searchBody = string(data)
			return respond(http.StatusOK, `{"places":[
				{"id":"far","displayName":{"text":"Hank's"},"location":{"latitude":36.5,"longitude":-79.5}},
				{"id":"`+hanksPlaceID+`","displayName":{"text":"Hank's Downtown Diner"},"formattedAddress":"1 Main St","location":{"latitude":35.7797,"longitude":-78.6383}}]}`, nil)
		}
		return nil
	}}

	place, err := linkClient(transport).ResolveMapsLink(context.Background(), "https://maps.app.goo.gl/abc123")
	if err != nil {
		t.Fatal(err)
	}
	if place.ID != hanksPlaceID || place.DisplayName.Text != "Hank's Downtown Diner" || place.Address != "1 Main St" {
		t.Fatalf("place = %#v", place)
	}
	var search struct {
		TextQuery    string `json:"textQuery"`
		LocationBias struct {
			Circle struct {
				Center Location `json:"center"`
			} `json:"circle"`
		} `json:"locationBias"`
	}
	if err := json.Unmarshal([]byte(searchBody), &search); err != nil {
		t.Fatalf("search body %q: %v", searchBody, err)
	}
	if center := search.LocationBias.Circle.Center; search.TextQuery != "Hank's Downtown Diner" || center.Latitude != 35.7797 || center.Longitude != -78.6383 {
		t.Fatalf("search body = %s, want the name biased around the pin 35.7797,-78.6383", searchBody)
	}
}

func TestResolveMapsLinkPlaceURLNeedsNoRedirect(t *testing.T) {
	transport := &linkTransport{handle: func(req *http.Request) *http.Response {
		if req.URL.Host == "places.googleapis.com" {
			return respond(http.StatusOK, `{"places":[{"id":"`+hanksPlaceID+`","displayName":{"text":"Hank's Downtown Diner"},"location":{"latitude":35.7797,"longitude":-78.6383}}]}`, nil)
		}
		return nil
	}}

	place, err := linkClient(transport).ResolveMapsLink(context.Background(), hanksPlaceURL)
	if err != nil {
		t.Fatal(err)
	}
	if place.ID != hanksPlaceID {
		t.Fatalf("place = %#v", place)
	}
	if hosts := transport.hosts(); len(hosts) != 1 || !hosts["places.googleapis.com"] {
		t.Fatalf("requested hosts = %v, want only the Places API", hosts)
	}
}

func TestResolveMapsLinkUsesPlaceIDWhenLinkHasOne(t *testing.T) {
	var detailsPath string
	transport := &linkTransport{handle: func(req *http.Request) *http.Response {
		detailsPath = req.URL.Path
		return respond(http.StatusOK, `{"id":"`+hanksPlaceID+`","displayName":{"text":"Hank's Downtown Diner"}}`, nil)
	}}

	place, err := linkClient(transport).ResolveMapsLink(context.Background(), "https://www.google.com/maps/search/?api=1&query=Hanks&query_place_id="+hanksPlaceID)
	if err != nil {
		t.Fatal(err)
	}
	if place.ID != hanksPlaceID || detailsPath != "/v1/places/"+hanksPlaceID {
		t.Fatalf("place = %#v, details path = %q", place, detailsPath)
	}
}

func TestResolveMapsLinkRejectsOtherHostsWithoutAnyRequest(t *testing.T) {
	for _, raw := range []string{
		"https://example.com/maps/place/Hanks",
		"https://maps.app.goo.gl.evil.example/abc",
		"https://evil.example/https://maps.app.goo.gl/abc",
		"https://www.google.com/search?q=hanks",
		"https://goo.gl/maps-other/abc",
		"https://google.com.evil.example/maps/place/Hanks",
		"https://user@maps.app.goo.gl/abc",
		"https://maps.app.goo.gl:8443/abc",
		"http://maps.app.goo.gl/abc",
		"ftp://maps.app.goo.gl/abc",
		"maps.app.goo.gl/abc",
		"not a url",
		"",
	} {
		t.Run(raw, func(t *testing.T) {
			transport := &linkTransport{handle: func(*http.Request) *http.Response { return nil }}
			_, err := linkClient(transport).ResolveMapsLink(context.Background(), raw)
			if !errors.Is(err, ErrNotMapsLink) {
				t.Fatalf("err = %v, want ErrNotMapsLink", err)
			}
			if len(transport.requests) != 0 {
				t.Fatalf("requests = %v, want none", transport.requests)
			}
		})
	}
}

func TestResolveMapsLinkDoesNotFollowRedirectToOtherHost(t *testing.T) {
	transport := &linkTransport{handle: func(req *http.Request) *http.Response {
		if req.URL.Host == "maps.app.goo.gl" {
			return redirect("https://evil.example/steal")
		}
		return nil
	}}

	_, err := linkClient(transport).ResolveMapsLink(context.Background(), "https://maps.app.goo.gl/abc")
	if !errors.Is(err, ErrNotMapsLink) {
		t.Fatalf("err = %v, want ErrNotMapsLink", err)
	}
	if hosts := transport.hosts(); len(hosts) != 1 || !hosts["maps.app.goo.gl"] {
		t.Fatalf("requested hosts = %v, want only maps.app.goo.gl", hosts)
	}
}

// rewriteTransport sends every request to an httptest server, whatever its host.
type rewriteTransport struct {
	target *url.URL
	base   http.RoundTripper
}

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = r.target.Scheme
	clone.URL.Host = r.target.Host
	return r.base.RoundTrip(clone)
}

func TestResolveMapsLinkMalformedRedirectLocationIsUnresolved(t *testing.T) {
	for _, location := range []string{"http://[::1", "https://maps.app.goo.gl/%zz"} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header()["Location"] = []string{location}
			w.WriteHeader(http.StatusFound)
		}))
		target := mustParse(t, server.URL)
		client := &Client{apiKey: "test-key", http: &http.Client{Transport: rewriteTransport{target: target, base: server.Client().Transport}}}

		_, err := client.ResolveMapsLink(context.Background(), "https://maps.app.goo.gl/abc")
		server.Close()
		if !errors.Is(err, ErrLinkUnresolved) {
			t.Fatalf("location %q: err = %v, want ErrLinkUnresolved", location, err)
		}
	}
}

func TestResolveMapsLinkStopsOnRedirectLoop(t *testing.T) {
	transport := &linkTransport{handle: func(req *http.Request) *http.Response {
		if req.URL.Path == "/a" {
			return redirect("https://maps.app.goo.gl/b")
		}
		return redirect("https://maps.app.goo.gl/a")
	}}

	_, err := linkClient(transport).ResolveMapsLink(context.Background(), "https://maps.app.goo.gl/a")
	if !errors.Is(err, ErrLinkUnresolved) || !strings.Contains(err.Error(), "loop") {
		t.Fatalf("err = %v, want redirect loop", err)
	}
	if len(transport.requests) != 2 {
		t.Fatalf("requests = %v, want 2 before the loop is caught", transport.requests)
	}
}

func TestResolveMapsLinkCapsLongRedirectChains(t *testing.T) {
	count := 0
	transport := &linkTransport{handle: func(req *http.Request) *http.Response {
		count++
		return redirect("https://maps.app.goo.gl/hop" + strings.Repeat("x", count))
	}}

	_, err := linkClient(transport).ResolveMapsLink(context.Background(), "https://maps.app.goo.gl/start")
	if !errors.Is(err, ErrLinkUnresolved) {
		t.Fatalf("err = %v, want ErrLinkUnresolved", err)
	}
	if len(transport.requests) != mapsLinkMaxRedirects {
		t.Fatalf("requests = %d, want %d", len(transport.requests), mapsLinkMaxRedirects)
	}
}

func TestResolveMapsLinkUnresolvable(t *testing.T) {
	tests := []struct {
		name    string
		handle  func(req *http.Request) *http.Response
		raw     string
		wantErr error
	}{
		{"short link that does not redirect", func(*http.Request) *http.Response { return respond(http.StatusOK, "<html>", nil) }, "https://maps.app.goo.gl/abc", ErrLinkUnresolved},
		{"short link that is gone", func(*http.Request) *http.Response { return respond(http.StatusNotFound, "", nil) }, "https://maps.app.goo.gl/abc", ErrLinkUnresolved},
		{"maps page without a place", func(*http.Request) *http.Response { return nil }, "https://www.google.com/maps", ErrLinkUnresolved},
		{"search page names no single place", func(*http.Request) *http.Response { return nil }, "https://www.google.com/maps/search/?api=1&query=pizza", ErrLinkUnresolved},
		{"q search names no single place", func(*http.Request) *http.Response { return nil }, "https://maps.google.com/?q=pizza", ErrLinkUnresolved},
		{"coordinates only", func(*http.Request) *http.Response { return nil }, "https://www.google.com/maps/place/35.7796,-78.6382/@35.7796,-78.6382,17z", ErrLinkUnresolved},
		{"no match near the link", func(*http.Request) *http.Response {
			return respond(http.StatusOK, `{"places":[{"id":"far","location":{"latitude":40,"longitude":-74}}]}`, nil)
		}, hanksPlaceURL, ErrLinkUnresolved},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := linkClient(&linkTransport{handle: tt.handle}).ResolveMapsLink(context.Background(), tt.raw)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolveMapsLinkShortLinkHostFailureIsNotUnresolved(t *testing.T) {
	for _, status := range []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			transport := &linkTransport{handle: func(*http.Request) *http.Response { return respond(status, "", nil) }}

			_, err := linkClient(transport).ResolveMapsLink(context.Background(), "https://maps.app.goo.gl/abc")

			if err == nil || errors.Is(err, ErrLinkUnresolved) || errors.Is(err, ErrNotMapsLink) {
				t.Fatalf("err = %v, want a lookup failure", err)
			}
		})
	}
}

func TestResolveMapsLinkPinComesOnlyFromTheDataSegment(t *testing.T) {
	const onPin = `{"places":[{"id":"onpin","displayName":{"text":"Hank's Downtown Diner"},"location":{"latitude":35.7797,"longitude":-78.6383}}]}`
	tests := []struct {
		name   string
		raw    string
		wantID string
	}{
		{"percent-encoded markers in the data segment", "https://www.google.com/maps/place/Hank's+Downtown+Diner/@35.7796,-78.6382,17z/data=!4m6!3m5%213d35.7797%214d-78.6383!16s%2Fg%2F1", "onpin"},
		{"markers in the place name segment", "https://www.google.com/maps/place/Hank's+!3d35.7797!4d-78.6383/@35.7796,-78.6382,17z", ""},
		{"encoded markers in the place name segment", "https://www.google.com/maps/place/Hank's+%213d35.7797%214d-78.6383/@35.7796,-78.6382,17z", ""},
		{"longitude with trailing junk", "https://www.google.com/maps/place/Hank's+Downtown+Diner/@35.7796,-78.6382,17z/data=!4m6!3m5!8m2!3d35.7797!4d-78.6383junk!16s%2Fg%2F1", ""},
		{"latitude with trailing junk", "https://www.google.com/maps/place/Hank's+Downtown+Diner/@35.7796,-78.6382,17z/data=!4m6!3m5!8m2!3d35.7797junk!4d-78.6383!16s%2Fg%2F1", ""},
		{"pin ending the data segment", "https://www.google.com/maps/place/Hank's+Downtown+Diner/@35.7796,-78.6382,17z/data=!4m6!3m5!8m2!3d35.7797!4d-78.6383", "onpin"},
		{"markers in the view segment", "https://www.google.com/maps/place/Hank's+Downtown+Diner/@!3d35.7797!4d-78.6383,17z", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &linkTransport{handle: func(*http.Request) *http.Response { return respond(http.StatusOK, onPin, nil) }}

			place, err := linkClient(transport).ResolveMapsLink(context.Background(), tt.raw)

			if tt.wantID == "" {
				if !errors.Is(err, ErrLinkUnresolved) || len(transport.requests) != 0 {
					t.Fatalf("place = %#v, err = %v, requests = %v, want ErrLinkUnresolved with no search", place, err, transport.requests)
				}
				return
			}
			if err != nil || place.ID != tt.wantID {
				t.Fatalf("place = %#v, err = %v, want %q", place, err, tt.wantID)
			}
		})
	}
}

func TestResolveMapsLinkPlaceNameWithoutCoordinatesIsRejected(t *testing.T) {
	// A chain's name alone would resolve to an arbitrary branch, so no search is made.
	transport := &linkTransport{handle: func(*http.Request) *http.Response {
		return respond(http.StatusOK, `{"places":[{"id":"`+hanksPlaceID+`","displayName":{"text":"Hank's Downtown Diner"},"location":{"latitude":35.7797,"longitude":-78.6383}}]}`, nil)
	}}

	_, err := linkClient(transport).ResolveMapsLink(context.Background(), "https://www.google.com/maps/place/Hank's+Downtown+Diner")
	if !errors.Is(err, ErrLinkUnresolved) {
		t.Fatalf("err = %v, want ErrLinkUnresolved", err)
	}
	if len(transport.requests) != 0 {
		t.Fatalf("requests = %v, want none", transport.requests)
	}
}

func TestResolveMapsLinkPlaceNameWithOnlyViewCoordinatesIsUnresolved(t *testing.T) {
	// The viewport center is not the restaurant's pin: centered near another
	// branch of the chain, it would prefill that branch.
	transport := &linkTransport{handle: func(*http.Request) *http.Response {
		return respond(http.StatusOK, `{"places":[{"id":"otherbranch","displayName":{"text":"Hank's Downtown Diner"},"location":{"latitude":35.7796,"longitude":-78.6382}}]}`, nil)
	}}

	place, err := linkClient(transport).ResolveMapsLink(context.Background(), "https://www.google.com/maps/place/Hank's+Downtown+Diner/@35.7796,-78.6382,17z")
	if !errors.Is(err, ErrLinkUnresolved) {
		t.Fatalf("place = %#v, err = %v, want ErrLinkUnresolved", place, err)
	}
	if len(transport.requests) != 0 {
		t.Fatalf("requests = %v, want none", transport.requests)
	}
}

func TestResolveMapsLinkChoosesAmongNearbyBranchesByPin(t *testing.T) {
	// Pin is 35.7797,-78.6383. 0.0043 degrees of latitude is about 0.3 miles and
	// 0.0007 about 0.05, so every branch below is inside the search radius.
	const (
		onPin    = `{"id":"onpin","displayName":{"text":"Hank's Downtown Diner"},"location":{"latitude":35.7797,"longitude":-78.6383}}`
		nearby   = `{"id":"nearby","displayName":{"text":"Hank's Downtown Diner"},"location":{"latitude":35.7840,"longitude":-78.6383}}`
		northish = `{"id":"north","displayName":{"text":"Hank's Downtown Diner"},"location":{"latitude":35.7804,"longitude":-78.6383}}`
		southish = `{"id":"south","displayName":{"text":"Hank's Downtown Diner"},"location":{"latitude":35.7790,"longitude":-78.6383}}`
		farther  = `{"id":"farther","displayName":{"text":"Hank's Downtown Diner"},"location":{"latitude":35.7740,"longitude":-78.6383}}`
	)
	tests := []struct {
		name    string
		results string
		wantID  string
	}{
		{"branch on the pin beats a more relevant one in range", nearby + "," + onPin, "onpin"},
		{"branch on the pin beats the rest whatever the order", onPin + "," + nearby, "onpin"},
		{"branch on the pin beats two others in range", nearby + "," + farther + "," + onPin, "onpin"},
		{"branches equally close to the pin are ambiguous", northish + "," + southish, ""},
		{"nobody on the pin is ambiguous", nearby + "," + farther, ""},
		{"unrelated restaurant beside the pin does not veto the linked one", onPin + "," + `{"id":"pizza","displayName":{"text":"Joe's Pizza"},"location":{"latitude":35.7800,"longitude":-78.6383}}`, "onpin"},
		{"unrelated restaurants do not break a tie either", `{"id":"pizza","displayName":{"text":"Joe's Pizza"},"location":{"latitude":35.7798,"longitude":-78.6383}}` + "," + onPin + "," + `{"id":"cafe","location":{"latitude":35.7796,"longitude":-78.6383}}`, "onpin"},
		{"similarly close branches are ambiguous even if one is on the pin", onPin + "," + `{"id":"beside","displayName":{"text":"Hank's Downtown Diner"},"location":{"latitude":35.7801,"longitude":-78.6383}}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &linkTransport{handle: func(*http.Request) *http.Response {
				return respond(http.StatusOK, `{"places":[`+tt.results+`]}`, nil)
			}}

			place, err := linkClient(transport).ResolveMapsLink(context.Background(), hanksPlaceURL)

			if tt.wantID == "" {
				if !errors.Is(err, ErrLinkUnresolved) {
					t.Fatalf("place = %#v, err = %v, want ErrLinkUnresolved", place, err)
				}
				return
			}
			if err != nil || place.ID != tt.wantID {
				t.Fatalf("place = %#v, err = %v, want %q", place, err, tt.wantID)
			}
		})
	}
}

func TestResolveMapsLinkSingleResultMustBeOnThePin(t *testing.T) {
	// Pin is 35.7797,-78.6383. A branch 0.0043 degrees north is about 0.3 miles
	// away: inside the search radius, but not the linked branch.
	tests := []struct {
		name   string
		result string
		wantID string
	}{
		{"same-name branch far from the pin", `{"id":"nearby","displayName":{"text":"Hank's Downtown Diner"},"location":{"latitude":35.7840,"longitude":-78.6383}}`, ""},
		{"same-name branch on the pin", `{"id":"onpin","displayName":{"text":"Hank's Downtown Diner"},"location":{"latitude":35.7797,"longitude":-78.6383}}`, "onpin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &linkTransport{handle: func(*http.Request) *http.Response {
				return respond(http.StatusOK, `{"places":[`+tt.result+`]}`, nil)
			}}

			place, err := linkClient(transport).ResolveMapsLink(context.Background(), hanksPlaceURL)

			if tt.wantID == "" {
				if !errors.Is(err, ErrLinkUnresolved) {
					t.Fatalf("place = %#v, err = %v, want ErrLinkUnresolved", place, err)
				}
				return
			}
			if err != nil || place.ID != tt.wantID {
				t.Fatalf("place = %#v, err = %v, want %q", place, err, tt.wantID)
			}
		})
	}
}

func TestResolveMapsLinkRequiresTheLinkedName(t *testing.T) {
	const onPin = `"location":{"latitude":35.7797,"longitude":-78.6383}`
	tests := []struct {
		name    string
		result  string
		wantID  string
		wantErr bool
	}{
		{"only nearby result is a different restaurant", `{"id":"other","displayName":{"text":"Joe's Pizza"},` + onPin + `}`, "", true},
		{"only nearby result has no name", `{"id":"other",` + onPin + `}`, "", true},
		{"name only shares a word", `{"id":"other","displayName":{"text":"Downtown Pizza"},` + onPin + `}`, "", true},
		{"name has the linked words in another order", `{"id":"other","displayName":{"text":"Diner Downtown Hank's"},` + onPin + `}`, "", true},
		{"same name ignoring case and punctuation", `{"id":"same","displayName":{"text":"HANKS downtown diner"},` + onPin + `}`, "same", false},
		{"linked name is part of the place's name", `{"id":"longer","displayName":{"text":"Hank's Downtown Diner & Bar"},` + onPin + `}`, "longer", false},
		{"place's name has a word added inside the linked name", `{"id":"famous","displayName":{"text":"Hank's Famous Downtown Diner"},` + onPin + `}`, "famous", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &linkTransport{handle: func(*http.Request) *http.Response {
				return respond(http.StatusOK, `{"places":[`+tt.result+`]}`, nil)
			}}

			place, err := linkClient(transport).ResolveMapsLink(context.Background(), hanksPlaceURL)

			if tt.wantErr {
				if !errors.Is(err, ErrLinkUnresolved) {
					t.Fatalf("place = %#v, err = %v, want ErrLinkUnresolved", place, err)
				}
				return
			}
			if err != nil || place.ID != tt.wantID {
				t.Fatalf("place = %#v, err = %v, want %q", place, err, tt.wantID)
			}
		})
	}
}

func TestNamesMatchUsesEachFoundWordOnce(t *testing.T) {
	tests := []struct {
		linked, found string
		want          bool
	}{
		{"Pizza Pizza", "Pizza Palace", false},
		{"Pizza Pizza", "Pizza Palace Pizza", true},
		{"Diner Hank's", "Hank's Diner", false},
		{"Hank's Diner", "Hanks Famous Diner", true},
	}
	for _, tt := range tests {
		if got := namesMatch(tt.linked, tt.found); got != tt.want {
			t.Errorf("namesMatch(%q, %q) = %v, want %v", tt.linked, tt.found, got, tt.want)
		}
	}
}

func TestResolveMapsLinkUnknownPlaceIDIsUnresolved(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{"404", http.StatusNotFound, `{"error":{"code":404,"status":"NOT_FOUND"}}`},
		{"404 without a body", http.StatusNotFound, ``},
		{"not found status", http.StatusBadRequest, `{"error":{"status":"NOT_FOUND"}}`},
		{"invalid request status", http.StatusBadRequest, `{"error":{"status":"INVALID_REQUEST"}}`},
		{"invalid argument for a malformed ID", http.StatusBadRequest, `{"error":{"code":400,"message":"Invalid place ID.","status":"INVALID_ARGUMENT"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &linkTransport{handle: func(*http.Request) *http.Response { return respond(tt.status, tt.body, nil) }}

			place, err := linkClient(transport).ResolveMapsLink(context.Background(), "https://www.google.com/maps/search/?api=1&query=Hanks&query_place_id="+hanksPlaceID)

			if !errors.Is(err, ErrLinkUnresolved) {
				t.Fatalf("place = %#v, err = %v, want ErrLinkUnresolved", place, err)
			}
		})
	}
}

func TestResolveMapsLinkCredentialErrorIsNotUnresolved(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"invalid API key", `{"error":{"code":400,"message":"API key not valid.","status":"INVALID_ARGUMENT","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"API_KEY_INVALID","domain":"googleapis.com"}]}}`},
		{"expired API key", `{"error":{"code":400,"status":"INVALID_ARGUMENT","details":[{"reason":"API_KEY_EXPIRED"}]}}`},
		{"unparsable body", `<html>INVALID_ARGUMENT`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := &linkTransport{handle: func(*http.Request) *http.Response { return respond(http.StatusBadRequest, tt.body, nil) }}

			_, err := linkClient(transport).ResolveMapsLink(context.Background(), "https://www.google.com/maps/search/?api=1&query=Hanks&query_place_id="+hanksPlaceID)

			if err == nil || errors.Is(err, ErrLinkUnresolved) {
				t.Fatalf("err = %v, want a lookup failure", err)
			}
		})
	}
}

func TestResolveMapsLinkDetailsServerErrorIsNotUnresolved(t *testing.T) {
	transport := &linkTransport{handle: func(*http.Request) *http.Response {
		return respond(http.StatusServiceUnavailable, `{"error":{"status":"UNAVAILABLE"}}`, nil)
	}}

	_, err := linkClient(transport).ResolveMapsLink(context.Background(), "https://www.google.com/maps/search/?api=1&query=Hanks&query_place_id="+hanksPlaceID)

	if err == nil || errors.Is(err, ErrLinkUnresolved) {
		t.Fatalf("err = %v, want a lookup failure", err)
	}
}

func TestResolveMapsLinkMapViewOnlyIsUnresolvedWithoutSearching(t *testing.T) {
	for _, raw := range []string{
		"https://www.google.com/maps/@35.7796,-78.6382,17z",
		"https://www.google.com/maps/place/@35.7796,-78.6382,17z",
	} {
		t.Run(raw, func(t *testing.T) {
			transport := &linkTransport{handle: func(*http.Request) *http.Response {
				return respond(http.StatusOK, `{"places":[{"id":"`+hanksPlaceID+`","displayName":{"text":"Hank's Downtown Diner"},"location":{"latitude":35.7796,"longitude":-78.6382}}]}`, nil)
			}}

			_, err := linkClient(transport).ResolveMapsLink(context.Background(), raw)

			if !errors.Is(err, ErrLinkUnresolved) {
				t.Fatalf("err = %v, want ErrLinkUnresolved", err)
			}
			if len(transport.requests) != 0 {
				t.Fatalf("requests = %v, want none", transport.requests)
			}
		})
	}
}

func TestResolveMapsLinkNeedsConfiguredClient(t *testing.T) {
	_, err := NewClient("").ResolveMapsLink(context.Background(), hanksPlaceURL)
	if err == nil || errors.Is(err, ErrNotMapsLink) || errors.Is(err, ErrLinkUnresolved) {
		t.Fatalf("err = %v, want a not-configured error", err)
	}
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestParseMapsReference(t *testing.T) {
	ref := parseMapsReference(mustParse(t, hanksPlaceURL))
	if ref.name != "Hank's Downtown Diner" || !ref.hasCoords || ref.latitude != 35.7797 || ref.longitude != -78.6383 {
		t.Fatalf("ref = %#v, want the name and the pin from !3d!4d", ref)
	}
	ref = parseMapsReference(mustParse(t, "https://www.google.com/maps/place/Caf%C3%A9+Luna/@35.5,-78.5,15z"))
	if ref.name != "Café Luna" || ref.hasCoords {
		t.Fatalf("ref = %#v, want the name and no pin: the view center is not one", ref)
	}
	ref = parseMapsReference(mustParse(t, "https://www.google.com/maps/place/@Pizza/@35.5,-78.5,15z"))
	if ref.name != "@Pizza" {
		t.Fatalf("ref = %#v, want a name that starts with @ kept", ref)
	}
	ref = parseMapsReference(mustParse(t, "https://www.google.com/maps?q=place_id:"+hanksPlaceID))
	if ref.placeID != hanksPlaceID || ref.name != "" {
		t.Fatalf("ref = %#v", ref)
	}
}
