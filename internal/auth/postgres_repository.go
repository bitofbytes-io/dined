package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) FindUserByOAuth(ctx context.Context, provider, providerID string) (*User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, email, name, avatar_url, oauth_provider, oauth_provider_id, created_at, updated_at, last_login_at
		FROM users
		WHERE oauth_provider = $1 AND oauth_provider_id = $2`, provider, providerID)
	return scanUser(row)
}

func (r *PostgresRepository) FindUserByEmail(ctx context.Context, email string) (*User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, email, name, avatar_url, oauth_provider, oauth_provider_id, created_at, updated_at, last_login_at
		FROM users
		WHERE email = $1`, email)
	return scanUser(row)
}

func (r *PostgresRepository) CreateUser(ctx context.Context, user User) (User, error) {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO users (id, email, name, avatar_url, oauth_provider, oauth_provider_id, created_at, updated_at, last_login_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		user.ID,
		user.Email,
		user.Name,
		user.AvatarURL,
		user.OAuthProvider,
		user.OAuthProviderID,
		user.CreatedAt,
		user.UpdatedAt,
		user.LastLoginAt,
	)
	return user, err
}

func (r *PostgresRepository) UpdateUserLogin(ctx context.Context, id uuid.UUID, name, avatarURL string) error {
	now := time.Now()
	_, err := r.pool.Exec(ctx, `
		UPDATE users
		SET name = $2, avatar_url = $3, last_login_at = $4, updated_at = $4
		WHERE id = $1`, id, name, avatarURL, now)
	return err
}

func (r *PostgresRepository) CreateSession(ctx context.Context, session Session, tokenHash string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_sessions (id, user_id, session_token_hash, expires_at, created_at, user_agent, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		session.ID,
		session.UserID,
		tokenHash,
		session.ExpiresAt,
		session.CreatedAt,
		session.UserAgent,
		session.IPAddress,
	)
	return err
}

func (r *PostgresRepository) FindSessionByTokenHash(ctx context.Context, tokenHash string) (*Session, *User, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT
			s.id, s.user_id, s.expires_at, s.created_at, s.user_agent, s.ip_address,
			u.id, u.email, u.name, u.avatar_url, u.oauth_provider, u.oauth_provider_id,
			u.created_at, u.updated_at, u.last_login_at
		FROM user_sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.session_token_hash = $1`, tokenHash)

	var session Session
	var user User
	err := row.Scan(
		&session.ID,
		&session.UserID,
		&session.ExpiresAt,
		&session.CreatedAt,
		&session.UserAgent,
		&session.IPAddress,
		&user.ID,
		&user.Email,
		&user.Name,
		&user.AvatarURL,
		&user.OAuthProvider,
		&user.OAuthProviderID,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastLoginAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	return &session, &user, nil
}

func (r *PostgresRepository) DeleteSession(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM user_sessions WHERE id = $1`, id)
	return err
}

func (r *PostgresRepository) DeleteExpiredSessions(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `DELETE FROM user_sessions WHERE expires_at < $1`, time.Now())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(row scanner) (*User, error) {
	var user User
	err := row.Scan(
		&user.ID,
		&user.Email,
		&user.Name,
		&user.AvatarURL,
		&user.OAuthProvider,
		&user.OAuthProviderID,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.LastLoginAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}
