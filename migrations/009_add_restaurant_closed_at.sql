-- +goose Up
-- +goose StatementBegin
-- The date a restaurant was marked permanently closed; NULL while it is open.
-- Closed restaurants keep their visits but leave the spinner, Want to Try and
-- Nearby.
ALTER TABLE restaurants ADD COLUMN closed_at DATE;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE restaurants DROP COLUMN closed_at;
-- +goose StatementEnd
