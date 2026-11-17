package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/google/uuid"
)

func getPublicYear(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	return getPublicYearFrom(t, repository.NewMemoryStore(), path)
}

func getPublicYearFrom(t *testing.T, store repository.DinerStore, path string) *httptest.ResponseRecorder {
	t.Helper()
	router, _ := newAuthenticatedTestRouter(t, store)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestRouterYearRendersYearWithVisits(t *testing.T) {
	// The demo visits are days old, so on January 1-2 they fall in the
	// previous year. Log one visit right now so the current year always has data.
	store := repository.NewMemoryStore()
	people, err := store.People(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateVisit(context.Background(), model.VisitInput{
		RestaurantName: "Year Test Diner",
		Address:        "1 Year Test Way",
		VisitedAt:      time.Now(),
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 8},
	}); err != nil {
		t.Fatal(err)
	}
	year := time.Now().In(apptime.EasternLocation()).Year()
	rec := getPublicYearFrom(t, store, "/year/"+strconv.Itoa(year))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, strconv.Itoa(year)+" in Review") || !strings.Contains(body, "Dines By Month") {
		t.Fatalf("status %d, want the demo year in review:\n%s", rec.Code, body)
	}
	if !strings.Contains(body, `href="/year/`+strconv.Itoa(year-1)+`"`) || strings.Contains(body, `href="/year/`+strconv.Itoa(year+1)+`"`) {
		t.Fatalf("current year should link back but not forward:\n%s", body)
	}
}

func TestRouterYearRendersYearWithoutVisits(t *testing.T) {
	rec := getPublicYear(t, "/year/1999")
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.Contains(body, "No dines logged in 1999.") || !strings.Contains(body, `href="/year/2000"`) {
		t.Fatalf("status %d, want an empty year in review:\n%s", rec.Code, body)
	}
}

func TestRouterYearRejectsInvalidYears(t *testing.T) {
	for _, path := range []string{"/year/abc", "/year/26", "/year/02026", "/year/1969", "/year/-001", "/year/+202"} {
		if rec := getPublicYear(t, path); rec.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", path, rec.Code)
		}
	}
}

func TestRouterTrophyLinksCurrentAndPreviousYear(t *testing.T) {
	year := time.Now().In(apptime.EasternLocation()).Year()
	rec := getPublicYear(t, "/trophy-case")
	body := rec.Body.String()
	for _, link := range []string{`href="/year/` + strconv.Itoa(year) + `"`, `href="/year/` + strconv.Itoa(year-1) + `"`} {
		if rec.Code != http.StatusOK || !strings.Contains(body, link) {
			t.Fatalf("status %d, trophy case missing %s:\n%s", rec.Code, link, body)
		}
	}
}
