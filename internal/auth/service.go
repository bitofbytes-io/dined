package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repo       Repository
	sessionTTL time.Duration
}

func NewService(repo Repository, sessionTTL time.Duration) *Service {
	if sessionTTL <= 0 {
		sessionTTL = 90 * 24 * time.Hour
	}
	return &Service{repo: repo, sessionTTL: sessionTTL}
}

func (s *Service) CreateOrUpdateUser(ctx context.Context, claims *GoogleClaims) (*User, error) {
	email := strings.ToLower(strings.TrimSpace(claims.Email))
	existing, err := s.repo.FindUserByOAuth(ctx, "google", claims.Sub)
	if err != nil {
		return nil, fmt.Errorf("find user by oauth: %w", err)
	}
	if existing == nil {
		existing, err = s.repo.FindUserByEmail(ctx, email)
		if err != nil {
			return nil, fmt.Errorf("find user by email: %w", err)
		}
	}

	if existing != nil {
		if err := s.repo.UpdateUserLogin(ctx, existing.ID, claims.Name, claims.Picture); err != nil {
			return nil, fmt.Errorf("update user login: %w", err)
		}
		existing.Name = claims.Name
		existing.AvatarURL = claims.Picture
		existing.LastLoginAt = time.Now()
		return existing, nil
	}

	now := time.Now()
	user := User{
		ID:              uuid.New(),
		Email:           email,
		Name:            claims.Name,
		AvatarURL:       claims.Picture,
		OAuthProvider:   "google",
		OAuthProviderID: claims.Sub,
		CreatedAt:       now,
		UpdatedAt:       now,
		LastLoginAt:     now,
	}
	created, err := s.repo.CreateUser(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return &created, nil
}

func (s *Service) CreateSession(ctx context.Context, userID uuid.UUID, userAgent, ipAddress string) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	now := time.Now()
	session := Session{
		ID:        uuid.New(),
		UserID:    userID,
		ExpiresAt: now.Add(s.sessionTTL),
		CreatedAt: now,
		UserAgent: truncate(userAgent, 512),
		IPAddress: truncate(ipAddress, 45),
	}
	if err := s.repo.CreateSession(ctx, session, hashToken(token)); err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	return token, nil
}

func (s *Service) ValidateSession(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, nil
	}
	session, user, err := s.repo.FindSessionByTokenHash(ctx, hashToken(token))
	if err != nil {
		return nil, fmt.Errorf("find session: %w", err)
	}
	if session == nil || user == nil {
		return nil, nil
	}
	if time.Now().After(session.ExpiresAt) {
		_ = s.repo.DeleteSession(ctx, session.ID)
		return nil, nil
	}
	return user, nil
}

func (s *Service) DeleteSession(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	session, _, err := s.repo.FindSessionByTokenHash(ctx, hashToken(token))
	if err != nil {
		return fmt.Errorf("find session: %w", err)
	}
	if session == nil {
		return nil
	}
	return s.repo.DeleteSession(ctx, session.ID)
}

func (s *Service) CleanupExpiredSessions(ctx context.Context) (int64, error) {
	return s.repo.DeleteExpiredSessions(ctx)
}

func (s *Service) SessionTTL() time.Duration {
	return s.sessionTTL
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func truncate(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}
