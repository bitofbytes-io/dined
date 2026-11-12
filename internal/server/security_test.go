package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/auth"
	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/google/uuid"
)

func TestRouterSecurityHeaders(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, err := store.People(ctx)
	if err != nil {
		t.Fatal(err)
	}
	photoJPEG := testJPEG(t)
	visitID, err := store.CreateVisit(ctx, model.VisitInput{
		RestaurantName: "Security Headers Diner",
		VisitedAt:      time.Now(),
		PickerID:       &people[0].ID,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{people[0].ID: 9},
		Photos:         []model.VisitPhotoInput{{DataURI: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(photoJPEG)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	visit, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	photoID := visit.Photos[0].ID.String()
	photoURL := "/photos/" + photoID

	authService := auth.NewService(auth.NewMemoryRepository(), time.Hour, nil)
	user, err := authService.CreateOrUpdateUser(ctx, &auth.GoogleClaims{
		Sub: "headers-test-user", Email: "family@example.com", EmailVerified: true, Name: "Family",
	})
	if err != nil {
		t.Fatal(err)
	}
	token, err := authService.CreateSession(ctx, user.ID, "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	for _, secureCookies := range []bool{false, true} {
		name := "local"
		if secureCookies {
			name = "secure"
		}
		t.Run(name, func(t *testing.T) {
			cfg := &config.Config{SecureCookies: secureCookies, AuthSessionTTL: time.Hour}
			router := New(cfg, store, places.NewClient(""), authService, nil).Router()
			for _, tc := range []struct {
				name          string
				method        string
				path          string
				status        int
				authenticated bool
				headers       map[string]string
				contentType   string
			}{
				{name: "public HTML", method: http.MethodGet, path: "/", status: http.StatusOK, contentType: "text/html; charset=utf-8"},
				{name: "login", method: http.MethodGet, path: "/login", status: http.StatusOK, contentType: "text/html; charset=utf-8"},
				{name: "authenticated form", method: http.MethodGet, path: "/log", status: http.StatusOK, authenticated: true, contentType: "text/html; charset=utf-8"},
				{name: "authentication redirect", method: http.MethodGet, path: "/log", status: http.StatusSeeOther},
				{name: "unauthorized write", method: http.MethodPost, path: "/visits", status: http.StatusUnauthorized, headers: map[string]string{"Origin": "http://example.com"}},
				{name: "cross-origin rejection", method: http.MethodPost, path: "/visits", status: http.StatusForbidden, authenticated: true, headers: map[string]string{"Origin": "https://other.example"}},
				{name: "OAuth callback rejection", method: http.MethodGet, path: "/api/auth/google/callback", status: http.StatusSeeOther},
				{name: "health", method: http.MethodGet, path: "/health", status: http.StatusOK, contentType: "text/plain; charset=utf-8"},
				{name: "readiness", method: http.MethodGet, path: "/ready", status: http.StatusOK, contentType: "application/json"},
				{name: "static asset", method: http.MethodGet, path: "/static/styles.css", status: http.StatusOK, contentType: "text/css; charset=utf-8"},
				{name: "favicon", method: http.MethodGet, path: "/favicon.ico", status: http.StatusOK},
				{name: "missing static asset", method: http.MethodGet, path: "/static/missing.css", status: http.StatusNotFound},
				{name: "not found", method: http.MethodGet, path: "/missing", status: http.StatusNotFound},
				{name: "method not allowed", method: http.MethodPost, path: "/health", status: http.StatusMethodNotAllowed, headers: map[string]string{"Origin": "http://example.com"}},
				{name: "invalid photo", method: http.MethodGet, path: "/photos/invalid", status: http.StatusBadRequest},
				{name: "missing photo", method: http.MethodGet, path: "/photos/" + uuid.NewString(), status: http.StatusNotFound},
				{name: "photo", method: http.MethodGet, path: photoURL, status: http.StatusOK, contentType: "image/jpeg"},
				{name: "cached photo", method: http.MethodGet, path: photoURL, status: http.StatusNotModified, headers: map[string]string{"If-None-Match": fmt.Sprintf(`"photo--%x"`, sha256.Sum256(photoJPEG))}},
				{name: "photo range", method: http.MethodGet, path: photoURL, status: http.StatusPartialContent, headers: map[string]string{"Range": "bytes=0-3"}, contentType: "image/jpeg"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					req := httptest.NewRequest(tc.method, tc.path, nil)
					for key, value := range tc.headers {
						req.Header.Set(key, value)
					}
					if tc.authenticated {
						req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
					}
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					if rec.Code != tc.status {
						t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
					}
					assertSecurityHeaders(t, rec.Result().Header, secureCookies)
					if tc.contentType != "" && rec.Header().Get("Content-Type") != tc.contentType {
						t.Errorf("Content-Type = %q, want %q", rec.Header().Get("Content-Type"), tc.contentType)
					}
					if tc.name == "authentication redirect" && rec.Header().Get("Location") != "/login?redirect=%2Flog" {
						t.Errorf("authentication redirect = %q", rec.Header().Get("Location"))
					}
					if tc.path == photoURL {
						if got := rec.Header().Get("Cache-Control"); got != "public, max-age=86400" {
							t.Errorf("photo Cache-Control = %q", got)
						}
						if got := rec.Header().Get("ETag"); got != fmt.Sprintf(`"photo--%x"`, sha256.Sum256(photoJPEG)) {
							t.Errorf("photo ETag = %q", got)
						}
						want := photoJPEG
						if tc.status == http.StatusNotModified {
							want = nil
						} else if tc.status == http.StatusPartialContent {
							want = photoJPEG[:4]
						}
						if !bytes.Equal(rec.Body.Bytes(), want) {
							t.Errorf("photo body differs: got %d bytes, want %d", rec.Body.Len(), len(want))
						}
					}
				})
			}

			for _, tc := range []struct {
				name   string
				store  repository.DinerStore
				status int
			}{
				{name: "degraded readiness", store: &failingPingStore{MemoryStore: store}, status: http.StatusServiceUnavailable},
				{name: "recovered panic", store: &panicPingStore{DinerStore: store}, status: http.StatusInternalServerError},
			} {
				t.Run(tc.name, func(t *testing.T) {
					router := New(cfg, tc.store, places.NewClient(""), authService, nil).Router()
					rec := getWithoutSession(t, router, "/ready")
					if rec.Code != tc.status {
						t.Fatalf("status = %d, want %d", rec.Code, tc.status)
					}
					assertSecurityHeaders(t, rec.Result().Header, secureCookies)
				})
			}

			t.Run("forwarded scheme does not control HSTS", func(t *testing.T) {
				for _, scheme := range []struct{ direct, forwarded string }{
					{direct: "http", forwarded: "https"},
					{direct: "https", forwarded: "http"},
				} {
					req := httptest.NewRequest(http.MethodGet, scheme.direct+"://example.com/health", nil)
					req.Header.Set("X-Forwarded-Proto", scheme.forwarded)
					rec := httptest.NewRecorder()
					router.ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Fatalf("status = %d, want 200", rec.Code)
					}
					assertSecurityHeaders(t, rec.Result().Header, secureCookies)
				}
			})
		})
	}
}

func assertSecurityHeaders(t *testing.T, headers http.Header, secureCookies bool) {
	t.Helper()
	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"X-Frame-Options":        "DENY",
	}
	if secureCookies {
		want["Strict-Transport-Security"] = "max-age=31536000"
	} else if values := headers.Values("Strict-Transport-Security"); len(values) != 0 {
		t.Errorf("local response includes HSTS: %v", values)
	}
	for name, value := range want {
		if values := headers.Values(name); len(values) != 1 || values[0] != value {
			t.Errorf("%s = %v, want one %q value", name, values, value)
		}
	}
}

type panicPingStore struct {
	repository.DinerStore
}

func (s *panicPingStore) Ping(context.Context) error {
	panic("test readiness panic")
}
