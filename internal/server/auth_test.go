package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/auth"
	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/middleware"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/repository"
)

type countingAuthRepository struct {
	*auth.MemoryRepository
	sessionLookups int
}

func (c *countingAuthRepository) FindSessionByTokenHash(ctx context.Context, tokenHash string) (*auth.Session, *auth.User, error) {
	c.sessionLookups++
	return c.MemoryRepository.FindSessionByTokenHash(ctx, tokenHash)
}

func TestRouterLooksUpSessionOncePerPage(t *testing.T) {
	repo := &countingAuthRepository{MemoryRepository: auth.NewMemoryRepository()}
	authService := auth.NewService(repo, time.Hour, nil)
	user, err := authService.CreateOrUpdateUser(context.Background(), &auth.GoogleClaims{Sub: "google-user", Email: "family@example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	token, err := authService.CreateSession(context.Background(), user.ID, "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	router := New(&config.Config{AuthSessionTTL: time.Hour}, repository.NewMemoryStore(), places.NewClient(""), authService, nil).Router()

	for _, path := range []string{"/log", "/dines"} {
		repo.sessionLookups = 0
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: middleware.CookieName, Value: token})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "LOGOUT") {
			t.Fatalf("%s did not render the signed-in navigation", path)
		}
		if repo.sessionLookups != 1 {
			t.Fatalf("%s looked up the session %d times, want 1", path, repo.sessionLookups)
		}
	}
}
