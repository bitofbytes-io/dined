-- +goose Up
-- +goose StatementBegin
-- A NULL picker means Everybody picked: an outing that is no one person's pick.
-- The foreign key to persons still applies to every non-NULL picker.
ALTER TABLE dining_visits
    ALTER COLUMN picked_by_person_id DROP NOT NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- This fails while any Everybody visit exists. Give those visits a person as
-- picker (or delete them) before rolling back.
ALTER TABLE dining_visits
    ALTER COLUMN picked_by_person_id SET NOT NULL;
-- +goose StatementEnd
