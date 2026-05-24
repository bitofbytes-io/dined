package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/auth"
)

func TestAuthAllowsPublicReadonlyHome(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	called := false
	Auth(testAuthService(t), false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})).ServeHTTP(rec, req)
	if !called {
		t.Fatal("handler was not called")
	}
}

func TestAuthAllowsPublicTrophyMapImage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/trophy-case/map.png", nil)
	rec := httptest.NewRecorder()
	called := false
	Auth(testAuthService(t), false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})).ServeHTTP(rec, req)
	if !called {
		t.Fatal("handler was not called")
	}
}

func TestAuthBlocksMutationsWithoutSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/visits", nil)
	rec := httptest.NewRecorder()
	Auth(testAuthService(t), false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	})).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got status %d", rec.Code)
	}
}

func TestAuthAllowsSessionCookie(t *testing.T) {
	authService, token := testAuthServiceWithSession(t)
	req := httptest.NewRequest(http.MethodPost, "/visits", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	rec := httptest.NewRecorder()
	called := false
	Auth(authService, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})).ServeHTTP(rec, req)
	if !called {
		t.Fatal("handler was not called")
	}
}

func TestSameOriginAllowsSafeMethodsWithoutOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://dined.example/dines", nil)
	rec := httptest.NewRecorder()

	SameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("got status %d", rec.Code)
	}
}

func TestSameOriginRejectsUnsafeMethodWithoutOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://dined.example/visits", nil)
	rec := httptest.NewRecorder()

	SameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("got status %d", rec.Code)
	}
}

func TestSameOriginAllowsForwardedHTTPSOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://dined.example/visits", nil)
	req.Host = "dined.example"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Origin", "https://dined.example")
	rec := httptest.NewRecorder()

	SameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("got status %d", rec.Code)
	}
}

func TestSameOriginAllowsRefererFallback(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://dined.example/visits", nil)
	req.Header.Set("Referer", "http://dined.example/log")
	rec := httptest.NewRecorder()

	SameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("got status %d", rec.Code)
	}
}

func TestSameOriginIgnoresInvalidForwardedProto(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "http://dined.example/visits", nil)
	req.Header.Set("X-Forwarded-Proto", "gopher")
	req.Header.Set("Origin", "http://dined.example")
	rec := httptest.NewRecorder()

	SameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("got status %d", rec.Code)
	}
}

func TestSameOriginRejectsCrossSiteOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://dined.example/visits", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()

	SameOrigin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("got status %d", rec.Code)
	}
}

func testAuthService(t *testing.T) *auth.Service {
	t.Helper()
	return auth.NewService(auth.NewMemoryRepository(), time.Hour)
}

func testAuthServiceWithSession(t *testing.T) (*auth.Service, string) {
	t.Helper()
	authService := testAuthService(t)
	user, err := authService.CreateOrUpdateUser(t.Context(), &auth.GoogleClaims{
		Sub:           "google-user-id",
		Email:         "family@example.com",
		EmailVerified: true,
		Name:          "Family",
	})
	if err != nil {
		t.Fatal(err)
	}
	token, err := authService.CreateSession(t.Context(), user.ID, "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	return authService, token
}
