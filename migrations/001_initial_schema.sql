-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE persons (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL UNIQUE,
    avatar_color TEXT NOT NULL,
    sort_order   INTEGER NOT NULL UNIQUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE restaurants (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name               TEXT NOT NULL,
    address            TEXT,
    latitude           DOUBLE PRECISION,
    longitude          DOUBLE PRECISION,
    phone              TEXT,
    website            TEXT,
    google_place_id    TEXT,
    google_rating      DOUBLE PRECISION,
    google_price_level INTEGER CHECK (google_price_level IS NULL OR google_price_level BETWEEN 1 AND 4),
    category           TEXT,
    is_chain           BOOLEAN NOT NULL DEFAULT false,
    metadata_json      JSONB,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_restaurants_google_place_id
    ON restaurants (google_place_id)
    WHERE google_place_id IS NOT NULL;
CREATE INDEX idx_restaurants_name ON restaurants (name);

CREATE TABLE dining_visits (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    restaurant_id       UUID NOT NULL REFERENCES restaurants(id) ON DELETE CASCADE,
    visited_at          TIMESTAMPTZ NOT NULL,
    picked_by_person_id UUID NOT NULL REFERENCES persons(id),
    price_level         INTEGER NOT NULL CHECK (price_level BETWEEN 1 AND 4),
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_dining_visits_restaurant_id ON dining_visits (restaurant_id);
CREATE INDEX idx_dining_visits_visited_at ON dining_visits (visited_at DESC);

CREATE TABLE visit_participant_ratings (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    visit_id   UUID NOT NULL REFERENCES dining_visits(id) ON DELETE CASCADE,
    person_id  UUID NOT NULL REFERENCES persons(id),
    rating     NUMERIC(3,1) NOT NULL CHECK (rating >= 0.5 AND rating <= 10.0 AND rating * 2 = floor(rating * 2)),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (visit_id, person_id)
);

CREATE INDEX idx_visit_participant_ratings_visit_id ON visit_participant_ratings (visit_id);

CREATE TABLE tags (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE visit_tags (
    visit_id UUID NOT NULL REFERENCES dining_visits(id) ON DELETE CASCADE,
    tag_id   UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (visit_id, tag_id)
);

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_restaurants_updated_at
    BEFORE UPDATE ON restaurants
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_dining_visits_updated_at
    BEFORE UPDATE ON dining_visits
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_visit_participant_ratings_updated_at
    BEFORE UPDATE ON visit_participant_ratings
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

INSERT INTO persons (name, avatar_color, sort_order) VALUES
    ('Daniel', '#0d6f6f', 1),
    ('Jen', '#c7332f', 2),
    ('Caleb', '#e5a72f', 3),
    ('Aiden', '#2f8f6d', 4);

INSERT INTO tags (name) VALUES
    ('Would Return'),
    ('Long Wait'),
    ('Great Service'),
    ('Overpriced'),
    ('Kid Approved');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS update_visit_participant_ratings_updated_at ON visit_participant_ratings;
DROP TRIGGER IF EXISTS update_dining_visits_updated_at ON dining_visits;
DROP TRIGGER IF EXISTS update_restaurants_updated_at ON restaurants;
DROP FUNCTION IF EXISTS update_updated_at_column();
DROP TABLE IF EXISTS visit_tags;
DROP TABLE IF EXISTS tags;
DROP TABLE IF EXISTS visit_participant_ratings;
DROP TABLE IF EXISTS dining_visits;
DROP TABLE IF EXISTS restaurants;
DROP TABLE IF EXISTS persons;
-- +goose StatementEnd
