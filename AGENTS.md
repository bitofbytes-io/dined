# Agent Guidance

- Edit `tailwind/styles.css`, not generated `static/styles.css`; run `make tail-prod` after CSS changes.
- Keep the memory store as an ephemeral preview path and use Postgres behavior as the production reference.
- Keep the Google Places key server-side. Do not render it into HTML or client JavaScript.
- Preserve the application-level email/domain allowlist in addition to Google OAuth configuration.
- Run `make test` so generated CSS synchronization is checked with the Go suite.
