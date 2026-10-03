package auth

import (
	"testing"
	"time"
)

func TestServiceCreatesAndValidatesSession(t *testing.T) {
	service := NewService(NewMemoryRepository(), time.Hour, nil)
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
	service := NewService(NewMemoryRepository(), time.Hour, nil)
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

func TestServiceRevokesSessionWhenEmailLeavesAllowlist(t *testing.T) {
	repo := NewMemoryRepository()
	allowlist := NewAllowlist([]string{"family@example.com"}, nil)
	service := NewService(repo, time.Hour, allowlist.Allowed)
	user, err := service.CreateOrUpdateUser(t.Context(), &GoogleClaims{Sub: "google-user-id", Email: "family@example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	token, err := service.CreateSession(t.Context(), user.ID, "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if validUser, err := service.ValidateSession(t.Context(), token); err != nil || validUser == nil {
		t.Fatalf("allowed session did not validate: user = %#v, err = %v", validUser, err)
	}

	// The same stored session, checked against an allowlist without the user.
	service = NewService(repo, time.Hour, NewAllowlist([]string{"someone-else@example.com"}, nil).Allowed)
	validUser, err := service.ValidateSession(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	if validUser != nil {
		t.Fatalf("removed user still validated: %#v", validUser)
	}
	if session, _, err := repo.FindSessionByTokenHash(t.Context(), hashToken(token)); err != nil || session != nil {
		t.Fatalf("revoked session was not deleted: session = %#v, err = %v", session, err)
	}
}
