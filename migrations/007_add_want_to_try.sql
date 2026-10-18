-- +goose Up
-- +goose StatementBegin
-- A place the family wants to try. Each restaurant is on the list at most
-- once; done_at is set when a dine is logged there, which takes it off the
-- list. A NULL suggester means it was Everybody's idea.
CREATE TABLE want_to_try (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id          UUID NOT NULL UNIQUE REFERENCES restaurants(id) ON DELETE CASCADE,
    suggested_by_person_id UUID REFERENCES persons(id),
    note                   TEXT,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    done_at                TIMESTAMPTZ
);

CREATE INDEX idx_want_to_try_open_created_at
    ON want_to_try (created_at DESC)
    WHERE done_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS want_to_try;
-- +goose StatementEnd
