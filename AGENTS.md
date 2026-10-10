# Agent Guidance

- Edit `static/styles.css` and `static/app.js` directly; they are served as-is, with no build step. Pass server values to `app.js` through `data-` attributes, not template actions.
- Keep the memory store as an ephemeral preview path and use Postgres behavior as the production reference.
- Keep the Google Places key server-side. Do not render it into HTML or client JavaScript.
- Preserve the application-level email/domain allowlist in addition to Google OAuth configuration.
- Use `make run` for the memory-backed local preview and `make run-postgres` when validating persistent Postgres behavior.
- With Goose installed and `DATABASE_URL` configured locally, use `make migrate`, `make migrate-status`, and `make migrate-down` for database changes.
- To include PostgreSQL regressions, point `DINED_TEST_DATABASE_URL` at a disposable database whose test account can create schemas and the `pgcrypto` extension; without it those tests skip. Match CI with `make test` and `go vet ./...`.
- `make test` includes browser tests that need Chrome or Chromium. Set `CHROME_BIN` or `CHROMIUM_BIN` to an executable to override discovery; without a valid override or a discoverable executable, those tests skip.
