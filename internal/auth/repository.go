package auth

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	FindUserByOAuth(ctx context.Context, provider, providerID string) (*User, error)
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	CreateUser(ctx context.Context, user User) (User, error)
	UpdateUserLogin(ctx context.Context, id uuid.UUID, name, avatarURL string) error

	CreateSession(ctx context.Context, session Session, tokenHash string) error
	FindSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, *User, error)
	DeleteSession(ctx context.Context, id uuid.UUID) error
	DeleteExpiredSessions(ctx context.Context) (int64, error)
}
