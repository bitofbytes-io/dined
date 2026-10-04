package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/google/uuid"
)

// testJPEG encodes a tiny real JPEG so photo responses can be decoded like a browser would.
func testJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for x := range 4 {
		for y := range 4 {
			img.Set(x, y, color.RGBA{R: 200, G: uint8(40 * x), B: uint8(40 * y), A: 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, nil); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func testJPEGDataURI(t *testing.T) string {
	t.Helper()
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(testJPEG(t))
}

// seedVisits adds count visits on top of the memory store's three demo visits.
func seedVisits(t *testing.T, store *repository.MemoryStore, count int) {
	t.Helper()
	ctx := context.Background()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := range count {
		_, err := store.CreateVisit(ctx, model.VisitInput{
			RestaurantName: fmt.Sprintf("Seed Diner %02d", i),
			VisitedAt:      time.Now().Add(-time.Duration(i) * time.Minute),
			PickerID:       &people[0].ID,
			PriceLevel:     2,
			Ratings:        map[uuid.UUID]float64{people[0].ID: 7},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func getPublic(router http.Handler, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestRouterDinesPaginatesTwentyPerPage(t *testing.T) {
	store := repository.NewMemoryStore()
	seedVisits(t, store, 22) // 25 visits in total.
	router, _ := newAuthenticatedTestRouter(t, store)

	first := getPublic(router, "/dines")
	if first.Code != http.StatusOK {
		t.Fatalf("page 1 status = %d", first.Code)
	}
	if got := strings.Count(first.Body.String(), `class="visit-card"`); got != 20 {
		t.Fatalf("page 1 visits = %d, want 20", got)
	}
	if !strings.Contains(first.Body.String(), `href="/dines?page=2" rel="next"`) || strings.Contains(first.Body.String(), `rel="prev"`) {
		t.Fatalf("page 1 pager links incorrect:\n%s", first.Body.String())
	}

	second := getPublic(router, "/dines?page=2")
	if second.Code != http.StatusOK {
		t.Fatalf("page 2 status = %d", second.Code)
	}
	if got := strings.Count(second.Body.String(), `class="visit-card"`); got != 5 {
		t.Fatalf("page 2 visits = %d, want 5", got)
	}
	if !strings.Contains(second.Body.String(), `href="/dines" rel="prev"`) || strings.Contains(second.Body.String(), `rel="next"`) {
		t.Fatalf("page 2 pager links incorrect:\n%s", second.Body.String())
	}

	if rec := getPublic(router, "/dines?page=3"); rec.Code != http.StatusNotFound {
		t.Fatalf("page past the end status = %d, want 404", rec.Code)
	}
	for _, page := range []string{"0", "-1", "abc", "1.5", "100001", "99999999999999999999"} {
		if rec := getPublic(router, "/dines?page="+page); rec.Code != http.StatusBadRequest {
			t.Errorf("page=%s status = %d, want 400", page, rec.Code)
		}
	}
}

func TestRouterDinesSinglePageHasNoPager(t *testing.T) {
	store := repository.NewMemoryStore()
	router, _ := newAuthenticatedTestRouter(t, store)

	rec := getPublic(router, "/dines")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), `class="pager"`) {
		t.Fatalf("single page should not render a pager:\n%s", rec.Body.String())
	}
	if rec := getPublic(router, "/dines?page=1"); rec.Code != http.StatusOK {
		t.Fatalf("explicit page 1 status = %d", rec.Code)
	}
}

func TestRouterServesPublicPhotosByURL(t *testing.T) {
	ctx := context.Background()
	photoJPEG := testJPEG(t)
	photoDataURI := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(photoJPEG)
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Photo Diner",
		VisitedAt:      time.Now(),
		PickerID:       &people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 9},
		Photos:         []model.VisitPhotoInput{{DataURI: photoDataURI}},
	})
	if err != nil {
		t.Fatal(err)
	}
	visit, err := store.Visit(ctx, *visitID)
	if err != nil || visit == nil || len(visit.Photos) != 1 {
		t.Fatalf("visit = %#v, err = %v", visit, err)
	}
	photoURL := "/photos/" + visit.Photos[0].ID.String()
	router, _ := newAuthenticatedTestRouter(t, store)

	for _, page := range []string{"/", "/dines", "/restaurants/" + visit.Restaurant.ID.String()} {
		rec := getPublic(router, page)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", page, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `<img src="`+photoURL+`"`) || strings.Contains(body, "base64,") {
			t.Fatalf("%s should reference photos by URL instead of embedding them:\n%s", page, body)
		}
	}

	rec := getPublic(router, photoURL)
	if rec.Code != http.StatusOK {
		t.Fatalf("photo status = %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("content type = %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=300" {
		t.Fatalf("cache control = %q, want a short max-age so deleted photos age out", got)
	}
	etag := rec.Header().Get("ETag")
	if etag != `"`+visit.Photos[0].ID.String()+`"` {
		t.Fatalf("etag = %q", etag)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("nosniff header = %q", got)
	}
	if !bytes.Equal(rec.Body.Bytes(), photoJPEG) {
		t.Fatalf("photo body is %d bytes, want the stored %d-byte JPEG", rec.Body.Len(), len(photoJPEG))
	}
	decoded, err := jpeg.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("served photo is not a decodable JPEG: %v", err)
	}
	if bounds := decoded.Bounds(); bounds.Dx() != 4 || bounds.Dy() != 4 {
		t.Fatalf("decoded photo bounds = %v", bounds)
	}

	revalidate := httptest.NewRequest(http.MethodGet, photoURL, nil)
	revalidate.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, revalidate)
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("revalidation status = %d, body %d bytes; want 304 with no body", rec.Code, rec.Body.Len())
	}

	if err := store.DeleteVisit(ctx, *visitID); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, revalidate)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("revalidating a deleted photo status = %d, want 404", rec.Code)
	}

	if rec := getPublic(router, "/photos/"+uuid.NewString()); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown photo status = %d, want 404", rec.Code)
	}
	if rec := getPublic(router, "/photos/not-a-uuid"); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid photo id status = %d, want 400", rec.Code)
	}
}

func TestRouterEditVisitKeepsInlinePhotoPreview(t *testing.T) {
	ctx := context.Background()
	photoDataURI := testJPEGDataURI(t)
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Edit Photo Diner",
		VisitedAt:      time.Now(),
		PickerID:       &people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 9},
		Photos:         []model.VisitPhotoInput{{DataURI: photoDataURI}},
	})
	if err != nil {
		t.Fatal(err)
	}
	visit, err := store.Visit(ctx, *visitID)
	if err != nil || visit == nil {
		t.Fatalf("visit = %#v, err = %v", visit, err)
	}
	router, token := newAuthenticatedTestRouter(t, store)
	req := httptest.NewRequest(http.MethodGet, "/visits/"+visitID.String()+"/edit", nil)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="keep_photo_id" value="`+visit.Photos[0].ID.String()+`"`) || !strings.Contains(body, `<img src="data:image/jpeg;base64,`) {
		t.Fatalf("edit form should keep existing photos:\n%s", body)
	}
}

func postVisitForm(t *testing.T, router http.Handler, token, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	setSameOrigin(req)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestRouterSavedVisitRedirectsToItsDinesPage(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	seedVisits(t, store, 22) // 25 visits; the demo visits are the oldest and land on page 2.
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	router, token := newAuthenticatedTestRouter(t, store)

	form := url.Values{}
	form.Set("restaurant_name", "Old Times Diner")
	form.Set("visited_at", "2020-01-02T18:00")
	form.Set("picker_id", people[0].ID.String())
	form.Set("price_level", "2")
	form.Set("rating_"+people[0].ID.String(), "8")
	rec := postVisitForm(t, router, token, "/visits", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create status = %d: %s", rec.Code, rec.Body.String())
	}
	location := rec.Header().Get("Location")
	oldID, ok := strings.CutPrefix(location, "/dines?page=2#")
	if !ok {
		t.Fatalf("create redirect = %q, want page 2 anchor", location)
	}
	if page := getPublic(router, "/dines?page=2"); !strings.Contains(page.Body.String(), `id="`+oldID+`"`) {
		t.Fatalf("page 2 does not contain the created visit %s", oldID)
	}

	visit, err := store.Visit(ctx, uuid.MustParse(oldID))
	if err != nil || visit == nil {
		t.Fatalf("visit = %#v, err = %v", visit, err)
	}
	form.Set("restaurant_id", visit.Restaurant.ID.String())
	form.Set("notes", "Edited")
	rec = postVisitForm(t, router, token, "/visits/"+oldID, form)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/dines?page=2#"+oldID {
		t.Fatalf("edit redirect = %d %q, want page 2 anchor", rec.Code, rec.Header().Get("Location"))
	}

	req := httptest.NewRequest(http.MethodGet, "/visits/"+oldID+"/edit", nil)
	req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), `href="/dines?page=2#`+oldID+`">Cancel</a>`) {
		t.Fatalf("edit cancel link should return to page 2:\n%s", rec.Body.String())
	}

	form.Set("restaurant_name", "Fresh Diner")
	form.Del("restaurant_id")
	form.Set("visited_at", time.Now().Format("2006-01-02T15:04"))
	rec = postVisitForm(t, router, token, "/visits", form)
	if location := rec.Header().Get("Location"); !strings.HasPrefix(location, "/dines#") {
		t.Fatalf("newest visit redirect = %q, want page 1 anchor", location)
	}
}
