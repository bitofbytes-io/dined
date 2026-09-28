package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/google/uuid"
)

const testPhotoDataURI = "data:image/jpeg;base64,aGVsbG8="

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
			PickerID:       people[0].ID,
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
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Photo Diner",
		VisitedAt:      time.Now(),
		PickerID:       people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 9},
		Photos:         []model.VisitPhotoInput{{DataURI: testPhotoDataURI}},
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
	if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=") || !strings.Contains(got, "public") {
		t.Fatalf("cache control = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("nosniff header = %q", got)
	}
	if rec.Body.String() != "hello" {
		t.Fatalf("photo body = %q, want decoded image bytes", rec.Body.String())
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
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Edit Photo Diner",
		VisitedAt:      time.Now(),
		PickerID:       people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 9},
		Photos:         []model.VisitPhotoInput{{DataURI: testPhotoDataURI}},
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
	if !strings.Contains(body, `name="keep_photo_id" value="`+visit.Photos[0].ID.String()+`"`) || !strings.Contains(body, `<img src="`+testPhotoDataURI+`"`) {
		t.Fatalf("edit form should keep existing photos:\n%s", body)
	}
}
