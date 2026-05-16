-- +goose Up
-- +goose StatementBegin
ALTER TABLE restaurants ADD COLUMN city TEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE restaurants DROP COLUMN city;
-- +goose StatementEnd
