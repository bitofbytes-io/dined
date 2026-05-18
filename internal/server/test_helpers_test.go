package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/auth"
	"github.com/bitofbytes-io/dined/internal/config"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/repository"
)

func newAuthenticatedTestRouter(t *testing.T, store repository.DinerStore) (http.Handler, string) {
	t.Helper()
	authService := auth.NewService(auth.NewMemoryRepository(), time.Hour)
	user, err := authService.CreateOrUpdateUser(context.Background(), &auth.GoogleClaims{
		Sub:           "test-google-user",
		Email:         "family@example.com",
		EmailVerified: true,
		Name:          "Family",
	})
	if err != nil {
		t.Fatal(err)
	}
	token, err := authService.CreateSession(context.Background(), user.ID, "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{SecureCookies: false, AuthSessionTTL: time.Hour}
	return New(cfg, store, places.NewClient(""), authService, nil).Router(), token
}
