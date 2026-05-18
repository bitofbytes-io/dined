package auth

import (
	"testing"
	"time"
)

func TestServiceCreatesAndValidatesSession(t *testing.T) {
	service := NewService(NewMemoryRepository(), time.Hour)
	user, err := service.CreateOrUpdateUser(t.Context(), &GoogleClaims{
		Sub:           "google-user-id",
		Email:         "Family@Example.com",
		EmailVerified: true,
		Name:          "Family",
	})
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "family@example.com" {
		t.Fatalf("got email %q", user.Email)
	}

	token, err := service.CreateSession(t.Context(), user.ID, "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	validUser, err := service.ValidateSession(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	if validUser == nil || validUser.ID != user.ID {
		t.Fatalf("got user %#v", validUser)
	}
}

func TestServiceDeletesSession(t *testing.T) {
	service := NewService(NewMemoryRepository(), time.Hour)
	user, err := service.CreateOrUpdateUser(t.Context(), &GoogleClaims{
		Sub:           "google-user-id",
		Email:         "family@example.com",
		EmailVerified: true,
		Name:          "Family",
	})
	if err != nil {
		t.Fatal(err)
	}
	token, err := service.CreateSession(t.Context(), user.ID, "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteSession(t.Context(), token); err != nil {
		t.Fatal(err)
	}
	validUser, err := service.ValidateSession(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	if validUser != nil {
		t.Fatalf("expected deleted session, got %#v", validUser)
	}
}
