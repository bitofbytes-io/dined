# Photo storage rollout and rollback

This release adds `original_bytes`, `thumbnail_bytes`, and `thumbnail_version` to
`visit_photos`. Migration 005 and `data_uri TEXT NOT NULL` remain intact. New
uploads write both forms; retained-photo edits only change ordering. IDs,
creation times, and original JPEG bytes are preserved. **Keeping the legacy
column means this release does not remove base64 storage overhead.** A later,
separately reviewed contract migration is required to reclaim that space.

No extra backup or backup job is part of this rollout. The existing weekly NAS
backup policy remains in place. Never run migration Down for image rollback.

## Image and request behavior

Uploads and legacy fallback reads accept JPEG only, up to 500 KiB, at most 8192
pixels on either axis and 16 million decoded pixels. JPEG configuration is
checked before pixel allocation; at most two full decode/thumbnail operations
run concurrently. Uploads are prepared once before store locks or transactions;
unchanged validated inputs reuse those original/thumbnail bytes across the
handler and storage boundary. `thumb-v1` is an aspect-preserving, never-upscaled, maximum
320-pixel JPEG, using deterministic integer area averaging and quality 80. The
browser's existing upload canvas normalizes submitted photographs before they
reach the server. The exact submitted original bytes are retained.

Saved-photo strips and edit previews request `/photos/{id}?variant=thumb-v1`;
the modal requests `/photos/{id}`. An old replica ignores the query parameter
and serves the original, so a mixed-version rollout still displays photos.
Visit/list/edit HTML contains metadata and URLs, not saved image data. Unsaved
form previews continue using their data URIs.

Each representation has a strong ETag derived from its actual bytes and variant.
GET/HEAD, conditional requests, and byte ranges use `http.ServeContent`. Successful
responses are public with `max-age=86400`; a deleted image may therefore remain
in a browser or intermediary cache for one day. Errors explicitly send `Cache-Control: no-store`, including invalid ranges.
A missing binary representation is derived from the validated legacy URI without
writing to the database. An invalid legacy image fails visibly; it is never
replaced with a placeholder or deleted.

## Required preflight (before any production migration)

1. Obtain the coordinator's authorization and use its established secure
   database execution path. Do not print a DSN or put it in argv, files, shell
   history, Git, or audit output. The utility uses the same database configuration as the app: `DATABASE_URL`,
   then `DATABASE_URL_FILE`, then `/run/secrets/dined_database_url`. In the
   deployed image, use the existing secret mount directly; do not export its
   contents into a shell or environment. It does not load OAuth/Places secrets.
2. Reconcile a complete read-only database inventory with every public photo:
   ID, visit ID, sort order, creation time, exact original length, and SHA-256.
   The public DOM inventory alone cannot establish database completeness.
3. Capture every existing photo in the real UI at 390×844, opening each original
   preview; keep working screenshots and hash manifests outside Git. Attach the required
   before/after screenshots to the PR through the user-authorized native upload
   path; never commit actual photos or database manifests. Confirm
   every legacy JPEG passes the bounded decoder. Stop on malformed data,
   unexpected rows, missing screenshots, or unexplained ID/hash differences.

The deployed image includes `/app/photo-storage`; it is never invoked by the
entrypoint. Alternatively build the operator utility from the reviewed revision
(it is not a background job and never applies schema migrations):

```sh
go build -o /PRIVATE/OPERATOR/DIRECTORY/photo-storage ./cmd/photo-storage
# Read-only, consistent snapshot; valid on the old or expanded schema.
/PRIVATE/OPERATOR/DIRECTORY/photo-storage -max-rows 1000 -timeout 5m \
  > /PRIVATE/EVIDENCE/before.jsonl 2> /PRIVATE/EVIDENCE/before-status.log
```

Only an exit code of zero plus `complete audit: photos=N` establishes a complete
audit. JSON lines contain metadata, dimensions, and SHA-256, never image bodies.
The audit refuses a count exceeding its cap rather than silently truncating. If
more than 1000 photos exist, review an explicit cap increase (hard maximum 10000).
It closes a bounded ID-list cursor first, then fully reads each photo before
decoding it in the same snapshot; image processing does not keep a database
query open. It uses a read-only repeatable-read transaction, 30-second statement timeout,
2-second lock timeout, and a five-minute default overall timeout.

## Expand, backfill, verify

After preflight approval, use the normal reviewed exact-digest deployment. App
startup applies additive migration 008 under the existing migration lock; there
is no automatic image backfill. Verify all replicas' deployed revision/digest
and health. While an old replica remains, its retained-photo delete/reinsert
logic can create legacy-only rows again.

Run the explicit bounded backfill through the approved operator execution path:

```sh
/PRIVATE/OPERATOR/DIRECTORY/photo-storage -apply -batch 25 -max-rows 1000 -timeout 5m \
  > /PRIVATE/EVIDENCE/backfill-audit.jsonl 2> /PRIVATE/EVIDENCE/backfill-status.log
```

Each row is locked and validated in its own transaction before the three binary
columns are filled. Legacy bytes and identity/order/timestamps are unchanged;
concurrent deletes are never resurrected. Batches restart from the lowest
missing UUID, including rows inserted behind a previous pass. At most the
specified number of rows are processed per run; an incomplete run returns
nonzero with its remaining count. Rerun safely after interruption or exhaustion.
Stop and investigate validation errors; never drop the legacy value to make a
backfill succeed.

**Run a final backfill after every old replica has stopped**, even if an earlier
pass reported zero. Then require a fresh read-only audit with every row showing
`binary_present=true` and `thumbnail_present=true`, and a zero remaining count.
Compare the before/after identity and original hash/length manifest, explicitly
reconciling legitimate concurrent user additions/deletions/edits.

Fetch every original HTTP response and verify its body matches the database
and preflight SHA-256; decode every thumbnail and confirm its dimensions. In a
fresh browser, capture every photo again at 390×844 and reconcile screenshot
coverage to the database, not just the first list page. Compare all originals
visually, verify strip thumbnails and full-size modal navigation, and attach the required before/after screenshots to the PR without committing
actual photo files. Keep database manifests and credentials private. Completion requires 100% coverage and no unexplained changes.

## Rollback

Roll back only the application image through the normal exact-digest process.
Keep migration 008, all binary columns, and the populated NOT NULL legacy column.
The previous binary can start on the expanded schema and uses the legacy URI.
It may recreate NULL binary fields when editing; this is expected. Re-upgrade
and rerun the bounded backfill plus complete audit after replicas converge.
Do not run Down, drop columns, restore backups, or perform bulk deletes as part
of an image rollback.
