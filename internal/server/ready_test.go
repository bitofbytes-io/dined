package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/repository"
)

// failingPingStore is a memory store whose database is unreachable.
type failingPingStore struct {
	*repository.MemoryStore
	deadline time.Duration
}

func (s *failingPingStore) Ping(ctx context.Context) error {
	if deadline, ok := ctx.Deadline(); ok {
		s.deadline = time.Until(deadline)
	}
	return errors.New("dial tcp 10.0.0.5:5432: connect: connection refused for user dined")
}

func getWithoutSession(t *testing.T, router http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestRouterHealthIsStaticAndPublic(t *testing.T) {
	router, _ := newAuthenticatedTestRouter(t, &failingPingStore{MemoryStore: repository.NewMemoryStore()})

	rec := getWithoutSession(t, router, "/health")

	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("got %d %q, want 200 \"ok\" even when the database is down", rec.Code, rec.Body.String())
	}
}

func TestRouterReadyReportsOKWhenDatabaseReachable(t *testing.T) {
	router, _ := newAuthenticatedTestRouter(t, repository.NewMemoryStore())

	rec := getWithoutSession(t, router, "/ready")

	if rec.Code != http.StatusOK {
		t.Fatalf("got status %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if body["status"] != "ok" || body["db"] != "ok" || len(body) != 2 {
		t.Fatalf("body = %v, want status and db ok", body)
	}
}

func TestRouterReadyReportsDegradedWhenDatabaseUnreachable(t *testing.T) {
	store := &failingPingStore{MemoryStore: repository.NewMemoryStore()}
	router, _ := newAuthenticatedTestRouter(t, store)

	rec := getWithoutSession(t, router, "/ready")

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("got status %d, want 503: %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if body["status"] != "degraded" || body["db"] != "error" || len(body) != 2 {
		t.Fatalf("body = %v, want status degraded and db error", body)
	}
	for _, leaked := range []string{"10.0.0.5", "connection refused", "dined"} {
		if strings.Contains(rec.Body.String(), leaked) {
			t.Fatalf("response leaked %q: %s", leaked, rec.Body.String())
		}
	}
	if store.deadline <= 0 || store.deadline > readyTimeout {
		t.Fatalf("ping deadline = %s, want within %s", store.deadline, readyTimeout)
	}
}
