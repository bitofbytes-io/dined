package auth

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
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

func TestScheduleSessionCleanupDeletesExpiredSessionsAtStartup(t *testing.T) {
	repo := NewMemoryRepository()
	service := NewService(repo, time.Hour, nil)
	user, err := service.CreateOrUpdateUser(t.Context(), &GoogleClaims{Sub: "google-user-id", Email: "family@example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	expired := Session{ID: uuid.New(), UserID: user.ID, CreatedAt: time.Now().Add(-2 * time.Hour), ExpiresAt: time.Now().Add(-time.Hour)}
	if err := repo.CreateSession(t.Context(), expired, "expired-hash"); err != nil {
		t.Fatal(err)
	}
	liveToken, err := service.CreateSession(t.Context(), user.ID, "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		service.ScheduleSessionCleanup(ctx, time.Hour)
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		session, _, err := repo.FindSessionByTokenHash(t.Context(), "expired-hash")
		if err != nil {
			t.Fatal(err)
		}
		if session == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expired session was not deleted at startup")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup scheduler did not stop after cancel")
	}
	if validUser, err := service.ValidateSession(t.Context(), liveToken); err != nil || validUser == nil {
		t.Fatalf("live session was removed: user = %#v, err = %v", validUser, err)
	}
}

func TestServiceLoginUpdatesChangedEmailForAllowlistCheck(t *testing.T) {
	repo := NewMemoryRepository()
	service := NewService(repo, time.Hour, NewAllowlist([]string{"new@example.com"}, nil).Allowed)
	if _, err := service.CreateOrUpdateUser(t.Context(), &GoogleClaims{Sub: "google-user-id", Email: "old@example.com", EmailVerified: true}); err != nil {
		t.Fatal(err)
	}

	// Same Google account (sub), new address: only the new address is allowed.
	user, err := service.CreateOrUpdateUser(t.Context(), &GoogleClaims{Sub: "google-user-id", Email: "New@Example.com", EmailVerified: true})
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "new@example.com" {
		t.Fatalf("user email = %q, want the new address", user.Email)
	}
	token, err := service.CreateSession(t.Context(), user.ID, "test", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if validUser, err := service.ValidateSession(t.Context(), token); err != nil || validUser == nil || validUser.Email != "new@example.com" {
		t.Fatalf("session for the new address did not validate: user = %#v, err = %v", validUser, err)
	}
	if byEmail, err := repo.FindUserByEmail(t.Context(), "new@example.com"); err != nil || byEmail == nil || byEmail.ID != user.ID {
		t.Fatalf("user not found by new email: %#v, err = %v", byEmail, err)
	}
}
