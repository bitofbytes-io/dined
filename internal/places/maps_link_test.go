package places

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
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
	if !strings.Contains(searchBody, `"textQuery":"Hank's Downtown Diner"`) || !strings.Contains(searchBody, "locationBias") {
		t.Fatalf("search body = %s", searchBody)
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

func TestResolveMapsLinkPlaceNameWithViewCoordinatesSearchesNearThem(t *testing.T) {
	var searchBody string
	transport := &linkTransport{handle: func(req *http.Request) *http.Response {
		data, _ := io.ReadAll(req.Body)
		searchBody = string(data)
		return respond(http.StatusOK, `{"places":[
			{"id":"otherbranch","displayName":{"text":"Hank's Downtown Diner"},"location":{"latitude":36.5,"longitude":-79.5}},
			{"id":"`+hanksPlaceID+`","displayName":{"text":"Hank's Downtown Diner"},"location":{"latitude":35.7797,"longitude":-78.6383}}]}`, nil)
	}}

	place, err := linkClient(transport).ResolveMapsLink(context.Background(), "https://www.google.com/maps/place/Hank's+Downtown+Diner/@35.7796,-78.6382,17z")
	if err != nil {
		t.Fatal(err)
	}
	if place.ID != hanksPlaceID {
		t.Fatalf("place = %#v, want the branch at the link's coordinates", place)
	}
	if !strings.Contains(searchBody, "locationBias") || !strings.Contains(searchBody, "35.7796") {
		t.Fatalf("search body = %s, want a bias on the link's coordinates", searchBody)
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
	if ref.name != "Café Luna" || ref.latitude != 35.5 || ref.longitude != -78.5 {
		t.Fatalf("ref = %#v, want the view center when there is no pin", ref)
	}
	ref = parseMapsReference(mustParse(t, "https://www.google.com/maps?q=place_id:"+hanksPlaceID))
	if ref.placeID != hanksPlaceID || ref.name != "" {
		t.Fatalf("ref = %#v", ref)
	}
}
