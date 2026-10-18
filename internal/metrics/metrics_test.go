package metrics

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

func scrape(t *testing.T, reg *prometheus.Registry) string {
	t.Helper()
	srv := httptest.NewServer(Handler(reg))
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
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /metrics = %d: %s", resp.StatusCode, body)
	}
	return string(body)
}

func assertContains(t *testing.T, body string, series ...string) {
	t.Helper()
	for _, s := range series {
		if !strings.Contains(body, s) {
			t.Errorf("metrics output is missing %s", s)
		}
	}
}

func TestMiddlewareLabelsByRoutePattern(t *testing.T) {
	r := chi.NewRouter()
	r.Use(Middleware)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {})
	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {})
	r.Get("/red-test/{id}", func(w http.ResponseWriter, r *http.Request) {})
	r.Route("/red-test/{id}/notes", func(r chi.Router) {
		r.Post("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "nope", http.StatusTeapot)
		})
	})

	for _, req := range []*http.Request{
		httptest.NewRequest(http.MethodGet, "/red-test/41?user=alice", nil),
		httptest.NewRequest(http.MethodGet, "/red-test/42", nil),
		httptest.NewRequest(http.MethodPost, "/red-test/42/notes/", nil),
		httptest.NewRequest(http.MethodGet, "/no-such-page/9f2c", nil),
		httptest.NewRequest("BREW", "/red-test/43", nil),
		httptest.NewRequest(http.MethodGet, "/health", nil),
		httptest.NewRequest(http.MethodGet, "/ready", nil),
	} {
		r.ServeHTTP(httptest.NewRecorder(), req)
	}

	body := scrape(t, NewRegistry())
	assertContains(t, body,
		`http_requests_total{code="200",method="GET",route="/red-test/{id}"} 2`,
		`http_requests_total{code="418",method="POST",route="/red-test/{id}/notes"} 1`,
		`http_requests_total{code="404",method="GET",route="unmatched"}`,
		`http_request_duration_seconds_bucket{code="200",method="GET",route="/red-test/{id}",le="+Inf"} 2`,
		`http_request_duration_seconds_count{code="200",method="GET",route="/red-test/{id}"} 2`,
		`http_requests_total{code="405",method="OTHER",route="unmatched"} 1`,
	)
	for _, leaked := range []string{"/red-test/41", "/red-test/42", "user=alice", "/no-such-page", `route="/health"`, `route="/ready"`, `method="BREW"`} {
		if strings.Contains(body, leaked) {
			t.Errorf("metrics output contains %q; routes must be labelled by pattern", leaked)
		}
	}
}

func TestRegistryExportsDomainAndRuntimeSeries(t *testing.T) {
	LoginFailure()
	OAuthFailure(ProviderGoogle)
	Uploads(2, ResultSuccess)
	Uploads(0, ResultFailure)
	Job(JobSessionCleanup, ResultSuccess)

	body := scrape(t, NewRegistry())
	assertContains(t, body,
		"dined_login_failures_total 1",
		`dined_oauth_failures_total{provider="google"} 1`,
		`dined_uploads_total{result="success"} 2`,
		`dined_uploads_total{result="failure"} 0`,
		`dined_jobs_total{job="session_cleanup",result="success"} 1`,
		`dined_jobs_total{job="session_cleanup",result="failure"} 0`,
		"go_goroutines ",
		"process_start_time_seconds ",
	)
}

func TestPoolCollectorExportsPoolStats(t *testing.T) {
	// pgxpool connects lazily, so an unreachable address still yields stats.
	pool, err := pgxpool.New(context.Background(), "postgres://dined:dined@127.0.0.1:1/dined?pool_max_conns=7")
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	body := scrape(t, NewRegistry(NewPoolCollector(pool)))
	assertContains(t, body,
		"pgxpool_max_conns 7",
		"pgxpool_acquired_conns 0",
		"pgxpool_idle_conns 0",
		"pgxpool_total_conns 0",
		"pgxpool_acquires_total 0",
		"pgxpool_empty_acquire_wait_seconds_total 0",
		"# TYPE pgxpool_acquire_duration_seconds_total counter",
	)
}

func TestHandlerServesOnlyMetrics(t *testing.T) {
	srv := httptest.NewServer(Handler(NewRegistry()))
	defer srv.Close()

	for path, want := range map[string]int{"/": http.StatusNotFound, "/health": http.StatusNotFound, "/metrics/x": http.StatusNotFound} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != want {
			t.Errorf("GET %s = %d, want %d", path, resp.StatusCode, want)
		}
	}
	resp, err := http.Post(srv.URL+"/metrics", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST /metrics = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}

func TestStartServesMetricsAndShutsDown(t *testing.T) {
	srv := Start("127.0.0.1:0", NewRegistry())
	if srv == nil {
		t.Fatal("Start returned nil for a free address")
	}
	url := "http://" + srv.Addr().String() + "/metrics"
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "dined_login_failures_total") {
		t.Fatalf("GET /metrics = %d: %.200s", resp.StatusCode, body)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := http.Get(url); err == nil {
		t.Fatal("metrics listener still serving after Shutdown")
	}
}

func TestStartIsOptional(t *testing.T) {
	if srv := Start(Disabled, NewRegistry()); srv != nil {
		t.Fatal("METRICS_ADDR=off should not start a listener")
	}

	taken, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	srv := Start(taken.Addr().String(), NewRegistry())
	if srv != nil {
		t.Fatal("Start should return nil when the address is in use")
	}
	// A nil server shuts down cleanly, so callers need no special case.
	if err := srv.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
