# Dined

Dined is a self-hosted restaurant memory ledger for recording visits, who chose a restaurant, who attended, ratings, photos, map-based history, a shared Want to Try list of places to go next, and a "Where should we eat?" spinner that picks from that list and old favorites and says whose turn it is to pick. It is a server-rendered Go application with Google sign-in.

## Requirements

- Docker 24+
- PostgreSQL 15+ for persistent deployments
- Goose CLI (optional) for manual migration status and rollbacks
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

Do not commit this file. `AUTH_GOOGLE_ALLOWED_DOMAINS` can replace or supplement the email allowlist. A domain entry only admits Google Workspace accounts whose `hd` (hosted domain) claim names that domain; list consumer addresses such as Gmail individually. Every request re-checks the signed-in user's email against the allowlist, so removing an address or domain ends that user's existing sessions.

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
| `METRICS_ADDR` | No | Address of the internal Prometheus listener; defaults to `:9464`, and `off` disables it. See [Metrics](#metrics) |
| `LOG_LEVEL` | No | Application log level; defaults to `info` |
| `TRUSTED_PROXY_CIDRS` | No | Comma-separated CIDRs of reverse proxies whose `X-Real-IP` / `X-Forwarded-For` headers are trusted for the client IP; defaults to loopback and `10.0.0.0/8` (Docker Swarm's default overlay pool, which holds Traefik's `proxy` network); set it to the proxy's network to narrow it further |

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

The SQL files in `migrations/` are embedded in the binary. With `DATA_STORE=postgres`, Dined applies any pending migrations at startup, before it serves traffic. If a migration fails, Dined exits non-zero and never becomes healthy, so a deploy whose migration fails does not finish rolling out. Each migration runs in its own transaction, so a failed migration leaves no partial changes and is not recorded. A Postgres advisory lock serializes startups, so replicas that start together apply each migration once. Versions are recorded in goose's standard `goose_db_version` table, so migrations already applied with the goose CLI are never re-run. No extra configuration is needed.


Startup allows 10 seconds to connect, then 60 seconds total for migrations,
including advisory-lock waiting. The lock is polled once per second for at most
30 seconds; releasing it has a separate 5-second cleanup allowance. Configure
the deployment healthcheck `start_period` to at least **90 seconds** in both
Dined and Learnd before deploying these limits. SIGTERM or SIGINT cancels startup;
a canceled or timed-out migration exits non-zero and logs the reason without
connection details. Transactional migrations roll back the interrupted migration;
previously completed migrations remain applied.

Migrations run as the runtime role from `DATABASE_URL`. It needs schema CREATE
and DDL/ownership rights for the app tables and `goose_db_version`, in addition
to normal application DML privileges. On a fresh Dined database, migration 001
also installs `pgcrypto`: provision that extension as an administrator first or
give the role the required database CREATE privilege for the trusted extension.
Do not assume a DML-only runtime role can migrate the database.

Table-rewriting DDL can hold ACCESS EXCLUSIVE locks while old replicas still
serve traffic. Schedule a maintenance window, stop/drain old replicas, apply the
migration explicitly, then start the new image for these changes. In particular,
Learnd migration 009 adds a stored generated search column and can rewrite
`entries`. Keep rollout migrations additive and compatible with the previous
image; use the manual migration path during maintenance for work that exceeds
the startup budget.

The goose CLI still works for inspecting or rolling back migrations by hand:

```bash
go install github.com/pressly/goose/v3/cmd/goose@latest
export DATABASE_URL='postgres://dined:change-me@localhost:5432/dined?sslmode=disable'
goose -dir migrations postgres "$DATABASE_URL" status
```

## Run with Docker

```bash
docker run --rm --name dined --network dined \
  --env-file .env \
  -p 4600:4600 \
  dined:local
```

Open <http://localhost:4600>. Two unauthenticated probe endpoints report status:

- <http://localhost:4600/health> returns `200 ok` whenever the process is up. It does not check dependencies.
- <http://localhost:4600/ready> pings the database with a 2 s timeout. It returns `200 {"status":"ok","db":"ok"}`, or `503 {"status":"degraded","db":"error"}` when the database is unreachable. The error itself is logged, not returned. With `DATA_STORE=memory` it always reports ok.

For an ephemeral preview without Postgres, set `DATA_STORE=memory` and omit the database URL and Places key. Google OAuth and an allowlist are still required.

## Metrics

`METRICS_ADDR` is trimmed and `off` is case-insensitive. Other values must be a valid IP address or hostname plus a numeric port from 0 to 65535 (IPv6 addresses need brackets). Invalid syntax stops startup; validation does not resolve hostnames. Keep the metrics port private and never publish it through Docker or the public proxy.

Dined serves Prometheus metrics at `GET /metrics` on a separate internal listener, `METRICS_ADDR` (default `:9464`). The public router on `PORT` never serves `/metrics`, so a public Traefik route cannot expose it. Scrape the listener over the internal overlay network, and do not publish the port. Set `METRICS_ADDR=off` to disable the listener. If the address cannot be bound, Dined logs the error and keeps serving. The listener shuts down gracefully with the app.

| Metric | Labels | Meaning |
| --- | --- | --- |
| `http_requests_total`, `http_request_duration_seconds` | `method`, `route`, `code` | Requests on the public router. `route` is the chi route pattern, such as `/restaurants/{id}`, or `unmatched`; raw paths and query strings are never labels. `/health`, `/ready`, and `/metrics` are not recorded |
| `pgxpool_*` | none | Postgres pool stats: acquired, idle, constructing, total, and max connections, plus acquire counts and wait time. Only with `DATA_STORE=postgres` |
| `dined_oauth_failures_total` | `provider` | Google OAuth callbacks that failed before a session was created, such as a bad state, a provider error, or a failed code exchange |
| `dined_login_failures_total` | none | Sign-ins refused for a Google account that is unverified or not on the allowlist |
| `dined_uploads_total` | `result` | Dine photos submitted with a create or edit, by `success` (stored) or `failure` (the save was rejected) |
| `dined_jobs_total` | `job`, `result` | Background job runs, currently `session_cleanup` |
| `go_*`, `process_*` | none | Go runtime and process collectors |

```bash
curl -s localhost:9464/metrics | grep '^dined_'
```

## Development

`make test` runs the Go race detector and requires a [supported platform](https://go.dev/doc/articles/race_detector#Requirements), a C compiler, and cgo enabled (`CGO_ENABLED=1`).

PostgreSQL regression tests use `DINED_TEST_DATABASE_URL` and create isolated schemas that are removed afterward. Point it at a disposable database; the test account needs permission to create schemas and the `pgcrypto` extension. Without this variable, PostgreSQL tests are skipped. CI supplies a PostgreSQL service for every pull request.

```bash
DINED_TEST_DATABASE_URL='postgres://dined:change-me@localhost:5432/dined_test?sslmode=disable' make test
```

```bash
cp local.mk.example local.mk
make run          # memory preview
make run-postgres # persistent local run
make test
```

`make run-postgres` applies pending migrations on startup. Use `make migrate`, `make migrate-status`, and `make migrate-down` for manual database maintenance with the goose CLI when `DATABASE_URL` is configured.
