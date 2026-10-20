-- +goose Up
-- Keep data_uri NOT NULL for old replicas and image-only rollback. Old writers
-- may reinsert rows with NULL binary columns; reads and the backfill handle that.
ALTER TABLE visit_photos
    ADD COLUMN original_bytes BYTEA,
    ADD COLUMN thumbnail_bytes BYTEA,
    ADD COLUMN thumbnail_version TEXT;

-- +goose Down
-- Forward-only: image rollback must retain both representations and photo data.
-- +goose StatementBegin
DO $$ BEGIN
    RAISE EXCEPTION 'Photo storage migration is forward-only; roll back the application image instead';
END $$;
-- +goose StatementEnd
