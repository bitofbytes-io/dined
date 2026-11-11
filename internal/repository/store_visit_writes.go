package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateVisit(ctx context.Context, input model.VisitInput) (*uuid.UUID, error) {
	input, photos, err := prepareVisitInput(ctx, input)
	if err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create visit: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := checkPicker(ctx, tx, input.PickerID); err != nil {
		return nil, err
	}
	restaurantID, err := resolveRestaurant(ctx, tx, input)
	if err != nil {
		return nil, err
	}

	var visitID uuid.UUID
	notes := nullableString(input.Notes)
	err = tx.QueryRow(ctx, `
		INSERT INTO dining_visits (restaurant_id, visited_at, picked_by_person_id, price_level, notes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`, restaurantID, input.VisitedAt, input.PickerID, input.PriceLevel, notes).Scan(&visitID)
	if err != nil {
		return nil, fmt.Errorf("create visit: %w", err)
	}

	if err := replaceVisitRatingsAndTags(ctx, tx, visitID, input); err != nil {
		return nil, err
	}

	if err := insertVisitPhotos(ctx, tx, visitID, photos, 0); err != nil {
		return nil, fmt.Errorf("create visit photos: %w", err)
	}

	if err := markWantToTryDone(ctx, tx, restaurantID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create visit: %w", err)
	}
	return &visitID, nil
}

// checkPicker returns model.ErrUnknownPicker when a picker is set but is not
// one of the people. A nil picker (Everybody) is always valid.
func checkPicker(ctx context.Context, tx pgx.Tx, pickerID *uuid.UUID) error {
	if pickerID == nil {
		return nil
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM persons WHERE id = $1)`, *pickerID).Scan(&exists); err != nil {
		return fmt.Errorf("check picker: %w", err)
	}
	if !exists {
		return model.ErrUnknownPicker
	}
	return nil
}

// resolveRestaurant returns the restaurant a new visit belongs to: the chosen
// restaurant, else one matched by Google place ID, else one matched by name and
// address, else a newly inserted one. A matched restaurant gains any details
// it is missing from the input.
func resolveRestaurant(ctx context.Context, tx pgx.Tx, input model.VisitInput) (uuid.UUID, error) {
	if input.RestaurantID != nil {
		if err := checkChosenRestaurantPlace(ctx, tx, *input.RestaurantID, input.GooglePlaceID); err != nil {
			return uuid.Nil, err
		}
		return *input.RestaurantID, fillRestaurantDetails(ctx, tx, *input.RestaurantID, withoutPlaceDetails(input))
	}

	var id uuid.UUID
	name := strings.TrimSpace(input.RestaurantName)
	address := strings.TrimSpace(input.Address)
	if placeID := nullableString(input.GooglePlaceID); placeID != nil {
		err := tx.QueryRow(ctx, `
			SELECT id
			FROM restaurants
			WHERE google_place_id = $1
			LIMIT 1`, *placeID).Scan(&id)
		if err == nil {
			return id, fillRestaurantDetails(ctx, tx, id, input)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, fmt.Errorf("find restaurant by place id: %w", err)
		}
	}
	if address != "" {
		err := tx.QueryRow(ctx, `
			SELECT id
			FROM restaurants
			WHERE lower(name) = lower($1)
			  AND lower(coalesce(address, '')) = lower($2)
			ORDER BY created_at
			LIMIT 1`, name, address).Scan(&id)
		if err == nil {
			return id, fillRestaurantDetails(ctx, tx, id, input)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, fmt.Errorf("find restaurant by name and address: %w", err)
		}
	}

	latitude, longitude, phone, website, googleRating, googlePriceLevel := googleMetadataValues(input.GoogleMetadata)
	err := tx.QueryRow(ctx, `
		INSERT INTO restaurants (
			name, address, city, latitude, longitude, phone, website, google_place_id,
			google_rating, google_price_level, category, is_chain
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (google_place_id) WHERE google_place_id IS NOT NULL DO UPDATE
		SET name = restaurants.name,
		    address = COALESCE(restaurants.address, EXCLUDED.address),
		    city = COALESCE(restaurants.city, EXCLUDED.city),
		    latitude = COALESCE(restaurants.latitude, EXCLUDED.latitude),
		    longitude = COALESCE(restaurants.longitude, EXCLUDED.longitude),
		    phone = COALESCE(restaurants.phone, EXCLUDED.phone),
		    website = COALESCE(restaurants.website, EXCLUDED.website),
		    google_rating = COALESCE(restaurants.google_rating, EXCLUDED.google_rating),
		    google_price_level = COALESCE(restaurants.google_price_level, EXCLUDED.google_price_level),
		    category = COALESCE(restaurants.category, EXCLUDED.category),
		    updated_at = NOW()
		RETURNING id`,
		name,
		nullableString(input.Address),
		nullableString(input.City),
		latitude,
		longitude,
		phone,
		website,
		nullableString(input.GooglePlaceID),
		googleRating,
		googlePriceLevel,
		nullableString(input.Category),
		input.IsChain,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create restaurant: %w", err)
	}
	return id, nil
}

// checkChosenRestaurantPlace returns a *model.PlaceConflictError when a visit
// chooses restaurant id but submits a Google place ID that is not its own:
// one another restaurant owns, or any other one when the chosen restaurant is
// already linked. The input's Places metadata describes that place, so it must
// not be filled onto the chosen restaurant.
func checkChosenRestaurantPlace(ctx context.Context, tx pgx.Tx, id uuid.UUID, googlePlaceID string) error {
	placeID := nullableString(googlePlaceID)
	if placeID == nil {
		return nil
	}
	var name string
	var chosenPlaceID *string
	// Lock the row so a concurrent visit that links this restaurant to a place
	// commits first and is seen here.
	err := tx.QueryRow(ctx, `SELECT name, google_place_id FROM restaurants WHERE id = $1 FOR UPDATE`, id).Scan(&name, &chosenPlaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find chosen restaurant: %w", err)
	}
	if chosenPlaceID != nil && *chosenPlaceID == *placeID {
		return nil
	}
	var owner string
	err = tx.QueryRow(ctx, `SELECT name FROM restaurants WHERE google_place_id = $1 AND id <> $2`, *placeID, id).Scan(&owner)
	if err == nil {
		return &model.PlaceConflictError{Restaurant: name, Owner: owner}
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("find place id owner: %w", err)
	}
	if chosenPlaceID != nil {
		return &model.PlaceConflictError{Restaurant: name}
	}
	return nil
}

// withoutPlaceDetails drops the Google-derived values a visit would fill onto
// the restaurant it chose when no place ID is submitted: the Places metadata
// and the hidden city field describe a place, so without one they are left
// over from another place (say, after a PlaceConflictError the user answered
// by clearing the place ID). The visible address and category stay: they are
// what the user sees and submits.
func withoutPlaceDetails(input model.VisitInput) model.VisitInput {
	if strings.TrimSpace(input.GooglePlaceID) == "" {
		input.GoogleMetadata = model.GoogleRestaurantMetadata{}
		input.City = ""
	}
	return input
}

// fillRestaurantDetails copies the input's address, city, place ID, category
// and Google metadata onto the restaurant wherever it has no value yet.
func fillRestaurantDetails(ctx context.Context, tx pgx.Tx, id uuid.UUID, input model.VisitInput) error {
	latitude, longitude, phone, website, googleRating, googlePriceLevel := googleMetadataValues(input.GoogleMetadata)
	_, err := tx.Exec(ctx, `
		UPDATE restaurants
		SET address = COALESCE(restaurants.address, $2),
		    city = COALESCE(restaurants.city, $3),
		    google_place_id = COALESCE(restaurants.google_place_id, $4),
		    latitude = COALESCE(restaurants.latitude, $5),
		    longitude = COALESCE(restaurants.longitude, $6),
		    phone = COALESCE(restaurants.phone, $7),
		    website = COALESCE(restaurants.website, $8),
		    google_rating = COALESCE(restaurants.google_rating, $9),
		    google_price_level = COALESCE(restaurants.google_price_level, $10),
		    category = COALESCE(restaurants.category, $11),
		    updated_at = NOW()
		WHERE id = $1`,
		id,
		nullableString(input.Address),
		nullableString(input.City),
		nullableString(input.GooglePlaceID),
		latitude,
		longitude,
		phone,
		website,
		googleRating,
		googlePriceLevel,
		nullableString(input.Category),
	)
	if err != nil {
		return fmt.Errorf("fill restaurant details: %w", err)
	}
	return nil
}

// replaceVisitRatingsAndTags writes the input's ratings and tags (creating
// NewTag if needed) as the visit's complete set.
func replaceVisitRatingsAndTags(ctx context.Context, tx pgx.Tx, visitID uuid.UUID, input model.VisitInput) error {
	tagIDs := append([]uuid.UUID(nil), input.TagIDs...)
	if name := strings.TrimSpace(input.NewTag); name != "" {
		var tagID uuid.UUID
		err := tx.QueryRow(ctx, `
			INSERT INTO tags (name)
			VALUES ($1)
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id`, name).Scan(&tagID)
		if err != nil {
			return fmt.Errorf("create tag: %w", err)
		}
		tagIDs = append(tagIDs, tagID)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM visit_participant_ratings WHERE visit_id = $1`, visitID); err != nil {
		return fmt.Errorf("clear visit ratings: %w", err)
	}
	for personID, score := range input.Ratings {
		_, err := tx.Exec(ctx, `
			INSERT INTO visit_participant_ratings (visit_id, person_id, rating)
			VALUES ($1, $2, $3)`, visitID, personID, score)
		if err != nil {
			return fmt.Errorf("write rating: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM visit_tags WHERE visit_id = $1`, visitID); err != nil {
		return fmt.Errorf("clear visit tags: %w", err)
	}
	for _, tagID := range tagIDs {
		_, err := tx.Exec(ctx, `
			INSERT INTO visit_tags (visit_id, tag_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, visitID, tagID)
		if err != nil {
			return fmt.Errorf("write visit tag: %w", err)
		}
	}
	return nil
}

func (s *Store) UpdateVisit(ctx context.Context, id uuid.UUID, input model.VisitInput) error {
	input, photos, err := prepareVisitInput(ctx, input)
	if err != nil {
		return err
	}
	if input.RestaurantID == nil {
		return errors.New("restaurant is required")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin update visit: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := checkPicker(ctx, tx, input.PickerID); err != nil {
		return err
	}
	var previousRestaurantID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT restaurant_id FROM dining_visits WHERE id = $1 FOR UPDATE`, id).Scan(&previousRestaurantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgx.ErrNoRows
	}
	if err != nil {
		return fmt.Errorf("find visit restaurant: %w", err)
	}
	notes := nullableString(input.Notes)
	result, err := tx.Exec(ctx, `
		UPDATE dining_visits
		SET restaurant_id = $2,
		    visited_at = $3,
		    picked_by_person_id = $4,
		    price_level = $5,
		    notes = $6
		WHERE id = $1`, id, *input.RestaurantID, input.VisitedAt, input.PickerID, input.PriceLevel, notes)
	if err != nil {
		return fmt.Errorf("update visit: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	if err := replaceVisitRatingsAndTags(ctx, tx, id, input); err != nil {
		return err
	}

	if err := reconcileVisitPhotos(ctx, tx, id, input, photos); err != nil {
		return fmt.Errorf("update visit photos: %w", err)
	}

	// A dine moved to another restaurant is now logged there. Editing a dine
	// in place leaves the list alone, so an older dine cannot clear a place
	// put back on it since.
	if previousRestaurantID != *input.RestaurantID {
		if err := markWantToTryDone(ctx, tx, *input.RestaurantID); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit update visit: %w", err)
	}
	return nil
}
