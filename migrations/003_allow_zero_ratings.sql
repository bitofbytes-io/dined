-- +goose Up
-- +goose StatementBegin
ALTER TABLE visit_participant_ratings
    DROP CONSTRAINT IF EXISTS visit_participant_ratings_rating_check,
    ADD CONSTRAINT visit_participant_ratings_rating_check
        CHECK (rating >= 0.0 AND rating <= 10.0 AND rating * 2 = floor(rating * 2));
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE visit_participant_ratings
    DROP CONSTRAINT IF EXISTS visit_participant_ratings_rating_check,
    ADD CONSTRAINT visit_participant_ratings_rating_check
        CHECK (rating >= 0.5 AND rating <= 10.0 AND rating * 2 = floor(rating * 2));
-- +goose StatementEnd
