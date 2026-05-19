-- +goose Up
-- +goose StatementBegin
CREATE TABLE visit_photos (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    visit_id     UUID NOT NULL REFERENCES dining_visits(id) ON DELETE CASCADE,
    data_uri     TEXT NOT NULL,
    content_type TEXT NOT NULL,
    byte_count   INTEGER NOT NULL CHECK (byte_count >= 0),
    sort_order   INTEGER NOT NULL CHECK (sort_order >= 0),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_visit_photos_visit_id_sort_order
    ON visit_photos (visit_id, sort_order);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS visit_photos;
-- +goose StatementEnd
