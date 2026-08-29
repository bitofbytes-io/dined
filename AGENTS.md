# Agent Guidance

- Edit `tailwind/styles.css`, not generated `static/styles.css`; run `make tail-prod` after CSS changes.
- Keep the memory store as an ephemeral preview path and use Postgres behavior as the production reference.
- Keep the Google Places key server-side. Do not render it into HTML or client JavaScript.
- Preserve the application-level email/domain allowlist in addition to Google OAuth configuration.
- Use `make run` for the memory-backed local preview and `make run-postgres` when validating persistent Postgres behavior.
- With Goose installed and `DATABASE_URL` configured locally, use `make migrate`, `make migrate-status`, and `make migrate-down` for database changes.
- `make test` depends on `make check-css`; run `make tail-prod` after Tailwind source changes so `static/styles.css` stays synchronized.
- Run `make test` so generated CSS synchronization is checked with the Go suite.
