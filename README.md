# Dined

Dined is a private, family-first restaurant memory ledger for tracking where the family ate, who picked it, who attended, and how everyone rated it.

## Tech Stack

- Go
- HTMX
- Tailwind-style CSS assets
- PostgreSQL
- Goose migrations
- Docker Swarm deployment

## Development

Run the no-database local preview:

```bash
make run
```

Open `http://localhost:4600`. Google OAuth is required for write access. Copy `local.mk.example` to
`local.mk`, then set:

```make
AUTH_GOOGLE_CLIENT_ID := your-local-client-id.apps.googleusercontent.com
AUTH_GOOGLE_CLIENT_SECRET := your-local-client-secret
AUTH_GOOGLE_REDIRECT_URL := http://localhost:4600/api/auth/google/callback
AUTH_GOOGLE_ALLOWED_EMAILS := you@gmail.com
```

The local Google OAuth client needs:

- Authorized JavaScript origin: `http://localhost:4600`
- Authorized redirect URI: `http://localhost:4600/api/auth/google/callback`

The default development mode uses `DATA_STORE=memory`, so changes persist only while the process is running.

## Production-Like Local Run

Create a local `local.mk` or pass environment variables with a Postgres URL and Google Places key:

```bash
make migrate
make run-postgres
```

Set `DATABASE_URL` in `local.mk` or the environment before running Postgres-backed targets.
Credentials in `DATABASE_URL` must already be URL-encoded before passing it to
`make`.

Run tests:

```bash
make test
```

## Deployment

The production deployment mirrors Dejaview:

- Image: `registry.bitofbytes.io/dined:<shortsha>`
- Service: `proxy_dined`
- Host: `https://dined.bitofbytes.io`
- Required Swarm secrets:
  - `dined_database_url`
  - `dined_google_places_api_key`
  - `dined_google_client_id`
  - `dined_google_client_secret`

The production Google OAuth client needs:

- Authorized JavaScript origin: `https://dined.bitofbytes.io`
- Authorized redirect URI: `https://dined.bitofbytes.io/api/auth/google/callback`

Dined also enforces its own `AUTH_GOOGLE_ALLOWED_EMAILS` allowlist. Google
Cloud test users can be configured as an additional control, but the app does
not rely on that setting to decide who can write.

Create the production database on bahamut:

```sql
create user dined with password '<strong-password>';
create database dined owner dined;
grant all privileges on database dined to dined;
```

Then store:

```text
postgres://dined:<strong-password>@192.168.1.2:8432/dined?sslmode=disable
```

in the `dined_database_url` Docker secret.

Create the OAuth Docker secrets on the Swarm manager:

```bash
printf '<google-oauth-client-id>' | docker secret create dined_google_client_id -
printf '<google-oauth-client-secret>' | docker secret create dined_google_client_secret -
```
