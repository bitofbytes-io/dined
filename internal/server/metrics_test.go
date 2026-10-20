package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bitofbytes-io/dined/internal/metrics"
	"github.com/bitofbytes-io/dined/internal/metrics/metricstest"
	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/bitofbytes-io/dined/internal/testutil"
	"github.com/google/uuid"
)

func TestRouterDoesNotServeMetrics(t *testing.T) {
	router, token := newAuthenticatedTestRouter(t, repository.NewMemoryStore())

	for _, cookie := range []string{"", token} {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: cookie})
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET /metrics on the public router = %d, want 404", rec.Code)
		}
	}
}

func TestRouterRecordsREDMetricsByRoutePattern(t *testing.T) {
	store := repository.NewMemoryStore()
	router, token := newAuthenticatedTestRouter(t, store)
	restaurantID := uuid.NewString()
	restaurantSeries := map[string]string{"method": "GET", "route": "/restaurants/{id}", "code": "404"}
	before := metricstest.Value(t, "http_requests_total", restaurantSeries)

	for _, path := range []string{"/restaurants/" + restaurantID, "/restaurants/" + uuid.NewString(), "/health", "/ready"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
		router.ServeHTTP(httptest.NewRecorder(), req)
	}

	if got := metricstest.Value(t, "http_requests_total", restaurantSeries) - before; got != 2 {
		t.Fatalf("GET /restaurants/{id} 404 count rose by %v, want 2", got)
	}
	body := scrapeMetrics(t)
	if strings.Contains(body, restaurantID) {
		t.Fatal("metrics output contains a raw restaurant ID; routes must be labelled by pattern")
	}
	for _, probe := range []string{`route="/health"`, `route="/ready"`} {
		if strings.Contains(body, probe) {
			t.Fatalf("metrics output records probe %s", probe)
		}
	}
}

func TestRouterCountsPhotoUploads(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	router, token := newAuthenticatedTestRouter(t, store)
	success := map[string]string{"result": metrics.ResultSuccess}
	failure := map[string]string{"result": metrics.ResultFailure}
	beforeSuccess, beforeFailure := metricstest.Value(t, "dined_uploads_total", success), metricstest.Value(t, "dined_uploads_total", failure)

	form := url.Values{}
	form.Set("restaurant_name", "Upload Counter Diner")
	form.Set("visited_at", "2026-05-17T20:50")
	form.Set("picker_id", people[0].ID.String())
	form.Set("price_level", "2")
	form.Set("rating_"+people[0].ID.String(), "8")
	form.Add("photo_data_uri", testutil.PhotoURI(1))
	form.Add("photo_data_uri", testutil.PhotoURI(2))
	if rec := postVisitForm(t, router, token, "/visits", form); rec.Code != http.StatusSeeOther {
		t.Fatalf("create visit = %d, want 303: %s", rec.Code, rec.Body.String())
	}

	form.Set("photo_data_uri", "data:image/png;base64,aGVsbG8=")
	if rec := postVisitForm(t, router, token, "/visits", form); rec.Code == http.StatusSeeOther {
		t.Fatal("create visit with a PNG photo succeeded, want a validation error")
	}

	// A photo is a failed upload even when a different field rejects the form.
	form.Set("photo_data_uri", testutil.PhotoURI(1))
	form.Set("visited_at", "not-a-date")
	if rec := postVisitForm(t, router, token, "/visits", form); rec.Code == http.StatusSeeOther {
		t.Fatal("create visit with an invalid date succeeded, want a validation error")
	}

	if got := metricstest.Value(t, "dined_uploads_total", success) - beforeSuccess; got != 2 {
		t.Fatalf("successful uploads rose by %v, want 2", got)
	}
	if got := metricstest.Value(t, "dined_uploads_total", failure) - beforeFailure; got != 2 {
		t.Fatalf("failed uploads rose by %v, want 2", got)
	}
}

func scrapeMetrics(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(metrics.Handler(metrics.NewRegistry()))
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
