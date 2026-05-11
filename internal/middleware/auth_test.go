package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthAllowsPublicReadonlyHome(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	called := false
	Auth("secret", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})).ServeHTTP(rec, req)
	if !called {
		t.Fatal("handler was not called")
	}
}

func TestAuthBlocksMutationsWithoutSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/visits", nil)
	rec := httptest.NewRecorder()
	Auth("secret", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	})).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("got status %d", rec.Code)
	}
}

func TestAuthAllowsBearerToken(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/visits", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	called := false
	Auth("secret", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})).ServeHTTP(rec, req)
	if !called {
		t.Fatal("handler was not called")
	}
}
