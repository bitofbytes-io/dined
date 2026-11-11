package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) Restaurants(ctx context.Context, q string) ([]model.Restaurant, error) {
	query := `
		SELECT id, name, address, city, latitude, longitude, phone, website, google_place_id,
		       google_rating, google_price_level, category, is_chain, closed_at, created_at, updated_at
		FROM restaurants
		WHERE $1 = '' OR name ILIKE '%' || $1 || '%' OR coalesce(address, '') ILIKE '%' || $1 || '%'
		ORDER BY name
		LIMIT 50`
	rows, err := s.pool.Query(ctx, query, strings.TrimSpace(q))
	if err != nil {
		return nil, fmt.Errorf("list restaurants: %w", err)
	}
	defer rows.Close()

	var restaurants []model.Restaurant
	for rows.Next() {
		restaurant, err := scanRestaurant(rows)
		if err != nil {
			return nil, err
		}
		restaurants = append(restaurants, restaurant)
	}
	return restaurants, rows.Err()
}

func (s *Store) Restaurant(ctx context.Context, id uuid.UUID) (*model.Restaurant, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT id, name, address, city, latitude, longitude, phone, website, google_place_id,
		       google_rating, google_price_level, category, is_chain, closed_at, created_at, updated_at
		FROM restaurants
		WHERE id = $1`, id)
	restaurant, err := scanRestaurant(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &restaurant, nil
}

// RestaurantsByPlaceIDs loads the saved restaurants linked to any of the
// Google place IDs, keyed by place ID. Unknown IDs are left out.
func (s *Store) RestaurantsByPlaceIDs(ctx context.Context, placeIDs []string) (map[string]model.Restaurant, error) {
	restaurants := map[string]model.Restaurant{}
	if len(placeIDs) == 0 {
		return restaurants, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, address, city, latitude, longitude, phone, website, google_place_id,
		       google_rating, google_price_level, category, is_chain, closed_at, created_at, updated_at
		FROM restaurants
		WHERE google_place_id = ANY($1)`, placeIDs)
	if err != nil {
		return nil, fmt.Errorf("restaurants by place ID: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		restaurant, err := scanRestaurant(rows)
		if err != nil {
			return nil, err
		}
		restaurants[*restaurant.GooglePlaceID] = restaurant
	}
	return restaurants, rows.Err()
}

func (s *Store) VisitedRestaurantMapPoints(ctx context.Context) ([]model.RestaurantMapPoint, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, r.name, r.latitude, r.longitude, COUNT(v.id), MAX(v.visited_at)
		FROM restaurants r
		JOIN dining_visits v ON v.restaurant_id = r.id
		WHERE r.latitude IS NOT NULL
		  AND r.longitude IS NOT NULL
		GROUP BY r.id, r.name, r.latitude, r.longitude
		ORDER BY MAX(v.visited_at) DESC, r.name`)
	if err != nil {
		return nil, fmt.Errorf("visited restaurant map points: %w", err)
	}
	defer rows.Close()

	var points []model.RestaurantMapPoint
	for rows.Next() {
		var point model.RestaurantMapPoint
		if err := rows.Scan(&point.RestaurantID, &point.Name, &point.Latitude, &point.Longitude, &point.VisitCount, &point.LatestVisitedAt); err != nil {
			return nil, fmt.Errorf("scan visited restaurant map point: %w", err)
		}
		points = append(points, point)
	}
	return points, rows.Err()
}

func (s *Store) UpdateRestaurantGoogleMetadata(ctx context.Context, id uuid.UUID, metadata model.GoogleRestaurantMetadata) error {
	latitude, longitude, phone, website, googleRating, googlePriceLevel := googleMetadataValues(metadata)
	category := nullableString(metadata.Category)
	_, err := s.pool.Exec(ctx, `
		UPDATE restaurants
		SET latitude = COALESCE($2, latitude),
		    longitude = COALESCE($3, longitude),
		    phone = COALESCE($4, phone),
		    website = COALESCE($5, website),
		    google_rating = COALESCE($6, google_rating),
		    google_price_level = COALESCE($7, google_price_level),
		    category = COALESCE($8, category),
		    updated_at = NOW()
		WHERE id = $1`, id, latitude, longitude, phone, website, googleRating, googlePriceLevel, category)
	if err != nil {
		return fmt.Errorf("update restaurant google metadata: %w", err)
	}
	return nil
}

func (s *Store) DeleteRestaurantIfUnvisited(ctx context.Context, id uuid.UUID) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		DELETE FROM restaurants r
		WHERE r.id = $1
		  AND NOT EXISTS (
		    SELECT 1
		    FROM dining_visits v
		    WHERE v.restaurant_id = r.id
		  )`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) UpdateRestaurant(ctx context.Context, id uuid.UUID, input model.RestaurantInput) error {
	if err := input.Validate(); err != nil {
		return err
	}

	result, err := s.pool.Exec(ctx, `
		UPDATE restaurants
		SET name = $2,
		    address = $3,
		    city = $4,
		    phone = $5,
		    website = $6,
		    google_rating = $7,
		    google_price_level = $8,
		    category = $9,
		    is_chain = $10,
		    closed_at = CASE WHEN $11 THEN COALESCE(closed_at, $12::date) ELSE NULL END
		WHERE id = $1`,
		id,
		strings.TrimSpace(input.Name),
		nullableString(input.Address),
		nullableString(input.City),
		nullableString(input.Phone),
		nullableString(input.Website),
		input.GoogleRating,
		input.GooglePriceLevel,
		nullableString(input.Category),
		input.IsChain,
		input.Closed,
		closingDate(time.Now()),
	)
	if err != nil {
		return fmt.Errorf("update restaurant: %w", err)
	}
	if result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) ToggleChain(ctx context.Context, id uuid.UUID, isChain bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE restaurants SET is_chain = $2 WHERE id = $1`, id, isChain)
	return err
}

// closingDate is the calendar date, in the app's Eastern time, a restaurant is
// marked closed on, as midnight UTC the way a DATE column reads back.
func closingDate(now time.Time) time.Time {
	year, month, day := now.In(apptime.EasternLocation()).Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

type restaurantScanner interface {
	Scan(dest ...any) error
}

func scanRestaurant(row restaurantScanner) (model.Restaurant, error) {
	var restaurant model.Restaurant
	err := row.Scan(
		&restaurant.ID,
		&restaurant.Name,
		&restaurant.Address,
		&restaurant.City,
		&restaurant.Latitude,
		&restaurant.Longitude,
		&restaurant.Phone,
		&restaurant.Website,
		&restaurant.GooglePlaceID,
		&restaurant.GoogleRating,
		&restaurant.GooglePriceLevel,
		&restaurant.Category,
		&restaurant.IsChain,
		&restaurant.ClosedAt,
		&restaurant.CreatedAt,
		&restaurant.UpdatedAt,
	)
	if err != nil {
		return model.Restaurant{}, fmt.Errorf("scan restaurant: %w", err)
	}
	return restaurant, nil
}

func googleMetadataValues(metadata model.GoogleRestaurantMetadata) (*float64, *float64, *string, *string, *float64, *int) {
	return metadata.Latitude,
		metadata.Longitude,
		nullableString(metadata.Phone),
		nullableString(metadata.Website),
		metadata.GoogleRating,
		metadata.GooglePriceLevel
}
