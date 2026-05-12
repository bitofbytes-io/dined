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

Open `http://localhost:4600`. The default local API token is:

```text
dined
```

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
  - `dined_api_token`
  - `dined_google_places_api_key`

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
