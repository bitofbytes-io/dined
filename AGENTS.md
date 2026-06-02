# Repository Guidelines

## Project Structure & Module Organization
- `cmd/dined` is the Go application entrypoint.
- `internal/` contains application code for handlers, auth, repositories, services, and supporting domain logic.
- `migrations/` stores Goose-managed PostgreSQL migrations.
- `tailwind/styles.css` is the source stylesheet; `static/styles.css` is generated output served by the app.
- `Dockerfile` builds the production container image.

## Build, Test, and Development Commands
- `make run` (alias for `make dev`): build CSS, then run a local memory-store preview on `PORT` with non-secure cookies for HTTP development.
- `make run-postgres`: build CSS, then run locally against Postgres; requires `DATABASE_URL` in `local.mk` or the environment.
- `make build`: build CSS and compile the production binary to `bin/dined`.
- `make tail-prod`: copy `tailwind/styles.css` to `static/styles.css`.
- `make check-css`: verify generated CSS is in sync with the source stylesheet.
- `make test`: run `make check-css`, then Go tests with `go test -v ./...`.
- `make migrate`, `make migrate-down`, and `make migrate-status`: apply, roll back, or inspect Goose migrations; require `DATABASE_URL`.
- `make docker-build`: build the Docker image locally.
- `make docker-buildx`: build and push the multi-arch Docker image; set `REGISTRY`, `IMAGE_REPO`, `PLATFORMS`, and `TAG` as needed.
- `make clean`: remove local build outputs under `bin/`.

## Coding Style & Naming Conventions
- Go code should stay `gofmt` formatted and follow the existing package boundaries under `internal/`.
- Keep CSS edits in `tailwind/styles.css`; regenerate `static/styles.css` with `make tail-prod`.
- Local configuration belongs in `local.mk` or environment variables, not committed files.

## Testing Guidelines
- Run `make test` before opening a PR so CSS sync and Go tests both run.
- For Postgres-backed changes, run the relevant Goose migration target with a local `DATABASE_URL` configured.

## Security & Configuration Tips
- Required local OAuth, database, and API key values must be configured locally or in CI; do not commit secrets.
- The default `DATA_STORE=memory` development path is a preview mode; use `make run-postgres` for production-like local behavior.
- The Google Places key is used server-side for map image generation and should not be rendered into browser HTML.

## Deployment Notes
- CI checks CSS sync, builds CSS, runs tests, builds/pushes the Docker image, then triggers deployment from the configured main-branch deployment target.
- Keep PRs explicit about migration and deployment impacts when changing database schema, auth, or container behavior.
