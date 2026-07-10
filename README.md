# Dined

Dined is a self-hosted restaurant memory ledger for recording visits, who chose a restaurant, who attended, ratings, photos, and map-based history. It is a server-rendered Go application with Google sign-in.

## Requirements

- Docker 24+
- PostgreSQL 15+ for persistent deployments
- Goose for database migrations
- A Google Cloud project with billing enabled
- An [OAuth 2.0 Web application client](https://developers.google.com/identity/protocols/oauth2/web-server)
- An API key with [Places API (New)](https://developers.google.com/maps/documentation/places/web-service/get-api-key) and [Maps Static API](https://developers.google.com/maps/documentation/maps-static/start) enabled

For local OAuth, add this exact authorized redirect URI to the Google client:

```text
http://localhost:4600/api/auth/google/callback
```

Production callbacks must use HTTPS and exactly match `AUTH_GOOGLE_REDIRECT_URL`. Restrict the Maps key to the required APIs and the server environments that will use it.

## Build the image

```bash
make tail-prod
docker build -t dined:local .
```

## Configure the application

Create the ignored `.env` file:

```dotenv
DATA_STORE=postgres
DATABASE_URL=postgres://dined:change-me@db:5432/dined?sslmode=disable
GOOGLE_PLACES_API_KEY=replace-with-your-api-key
AUTH_GOOGLE_CLIENT_ID=replace-with-your-client-id.apps.googleusercontent.com
AUTH_GOOGLE_CLIENT_SECRET=replace-with-your-client-secret
AUTH_GOOGLE_REDIRECT_URL=http://localhost:4600/api/auth/google/callback
AUTH_GOOGLE_ALLOWED_EMAILS=you@example.com
SECURE_COOKIES=false
PORT=4600
```

Do not commit this file. `AUTH_GOOGLE_ALLOWED_DOMAINS` can replace or supplement the email allowlist.

| Setting | Required | Purpose |
| --- | --- | --- |
| `DATA_STORE` | No | `postgres` for persistence or `memory` for an ephemeral preview |
| `DATABASE_URL` | With Postgres | PostgreSQL connection string |
| `GOOGLE_PLACES_API_KEY` | With Postgres | Places search/details and server-rendered static maps |
| `AUTH_GOOGLE_CLIENT_ID` | Yes | Google OAuth client ID |
| `AUTH_GOOGLE_CLIENT_SECRET` | Yes | Google OAuth client secret |
| `AUTH_GOOGLE_ALLOWED_EMAILS` or `AUTH_GOOGLE_ALLOWED_DOMAINS` | Yes | Login allowlist |
| `AUTH_GOOGLE_REDIRECT_URL` | No | OAuth callback; defaults to the local URL above |
| `AUTH_SESSION_TTL` | No | Go duration for sessions; defaults to `2160h` |
| `SECURE_COOKIES` | No | Set `false` for local HTTP; defaults to `true` |
| `PORT` | No | HTTP port; defaults to `4600` |
| `LOG_LEVEL` | No | Application log level; defaults to `info` |

The database URL, Places key, OAuth client ID, and OAuth secret support corresponding `*_FILE` variables and default secret paths under `/run/secrets/dined_*`.

## Database and migrations

```bash
docker network create dined

docker run -d --name db --network dined \
  -e POSTGRES_DB=dined \
  -e POSTGRES_USER=dined \
  -e POSTGRES_PASSWORD=change-me \
  -p 5432:5432 \
  -v dined-postgres:/var/lib/postgresql/data \
  postgres:17

until docker exec db pg_isready -U dined -d dined >/dev/null 2>&1; do sleep 1; done
```

Apply migrations before starting Dined:

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
export DATABASE_URL='postgres://dined:change-me@localhost:5432/dined?sslmode=disable'
goose -dir migrations postgres "$DATABASE_URL" up
```

## Run with Docker

```bash
docker run --rm --name dined --network dined \
  --env-file .env \
  -p 4600:4600 \
  dined:local
```

Open <http://localhost:4600>. The health endpoint is <http://localhost:4600/health>.

For an ephemeral preview without Postgres, set `DATA_STORE=memory` and omit the database URL and Places key. Google OAuth and an allowlist are still required.

## Development

```bash
cp local.mk.example local.mk
make run          # memory preview
make run-postgres # persistent local run
make test
```

Use `make migrate`, `make migrate-status`, and `make migrate-down` for database maintenance when `DATABASE_URL` is configured.
