-- +goose Up
-- +goose StatementBegin
CREATE TABLE users (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email             TEXT NOT NULL,
    name              TEXT NOT NULL DEFAULT '',
    avatar_url        TEXT NOT NULL DEFAULT '',
    oauth_provider    TEXT NOT NULL,
    oauth_provider_id TEXT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_login_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_users_email ON users (email);
CREATE UNIQUE INDEX idx_users_oauth ON users (oauth_provider, oauth_provider_id);

CREATE TABLE user_sessions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_token_hash TEXT NOT NULL UNIQUE,
    expires_at         TIMESTAMPTZ NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    user_agent         TEXT NOT NULL DEFAULT '',
    ip_address         TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_user_sessions_user_id ON user_sessions (user_id);
CREATE INDEX idx_user_sessions_expires_at ON user_sessions (expires_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS users;
-- +goose StatementEnd
