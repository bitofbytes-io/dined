package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

type DinerStore interface {
	People(context.Context) ([]model.Person, error)
	Tags(context.Context) ([]model.Tag, error)
	Restaurants(context.Context, string) ([]model.Restaurant, error)
	Restaurant(context.Context, uuid.UUID) (*model.Restaurant, error)
	Visit(context.Context, uuid.UUID) (*model.Visit, error)
	Visits(context.Context, int) ([]model.Visit, error)
	VisitsPage(ctx context.Context, limit, offset int) ([]model.Visit, error)
	VisitPosition(context.Context, uuid.UUID) (int, bool, error)
	VisitPhoto(context.Context, uuid.UUID) (*model.VisitPhoto, error)
	RestaurantVisits(context.Context, uuid.UUID) ([]model.Visit, error)
	RestaurantVisitSummaries(context.Context, []uuid.UUID) (map[uuid.UUID][]model.Visit, error)
	VisitedRestaurantMapPoints(context.Context) ([]model.RestaurantMapPoint, error)
	CreateVisit(context.Context, model.VisitInput) (*uuid.UUID, error)
	UpdateVisit(context.Context, uuid.UUID, model.VisitInput) error
	UpdateRestaurantGoogleMetadata(context.Context, uuid.UUID, model.GoogleRestaurantMetadata) error
	DeleteVisit(context.Context, uuid.UUID) error
	DeleteRestaurantIfUnvisited(context.Context, uuid.UUID) (bool, error)
	UpdateRestaurant(context.Context, uuid.UUID, model.RestaurantInput) error
	ToggleChain(context.Context, uuid.UUID, bool) error
	Stats(context.Context) (model.Stats, error)
	PickerTurn(context.Context) (model.PickerTurn, error)
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) People(ctx context.Context) ([]model.Person, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, avatar_color FROM persons ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("list people: %w", err)
	}
	defer rows.Close()

	var people []model.Person
	for rows.Next() {
		var person model.Person
		if err := rows.Scan(&person.ID, &person.Name, &person.AvatarColor); err != nil {
			return nil, fmt.Errorf("scan person: %w", err)
		}
		people = append(people, person)
	}
	return people, rows.Err()
}

func (s *Store) Tags(ctx context.Context) ([]model.Tag, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name FROM tags ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list tags: %w", err)
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		var tag model.Tag
		if err := rows.Scan(&tag.ID, &tag.Name); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (s *Store) Restaurants(ctx context.Context, q string) ([]model.Restaurant, error) {
	query := `
		SELECT id, name, address, city, latitude, longitude, phone, website, google_place_id,
		       google_rating, google_price_level, category, is_chain, created_at, updated_at
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
		       google_rating, google_price_level, category, is_chain, created_at, updated_at
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

func (s *Store) Visit(ctx context.Context, id uuid.UUID) (*model.Visit, error) {
	rows, err := s.pool.Query(ctx, visitSelectSQL()+` WHERE v.id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("visit: %w", err)
	}
	defer rows.Close()

	visits, err := s.scanVisits(ctx, rows, withPhotoData)
	if err != nil {
		return nil, err
	}
	if len(visits) == 0 {
		return nil, nil
	}
	return &visits[0], nil
}

func (s *Store) Visits(ctx context.Context, limit int) ([]model.Visit, error) {
	return s.VisitsPage(ctx, limit, 0)
}

// VisitsPage lists visits newest first. A limit of zero returns every visit after offset.
// Photos carry metadata only; their image bytes are served by VisitPhoto.
func (s *Store) VisitsPage(ctx context.Context, limit, offset int) ([]model.Visit, error) {
	query := visitSelectSQL() + ` ORDER BY v.visited_at DESC, v.created_at DESC, v.id OFFSET $1`
	args := []any{max(offset, 0)}
	if limit > 0 {
		query += ` LIMIT $2`
		args = append(args, limit)
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list visits: %w", err)
	}
	defer rows.Close()
	return s.scanVisits(ctx, rows, withPhotoMetadata)
}

// VisitPosition returns the visit's zero-based index in VisitsPage ordering, or false if it does not exist.
func (s *Store) VisitPosition(ctx context.Context, id uuid.UUID) (int, bool, error) {
	var position int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(v.id)
		FROM dining_visits t
		LEFT JOIN dining_visits v
		  ON v.visited_at > t.visited_at
		  OR (v.visited_at = t.visited_at AND v.created_at > t.created_at)
		  OR (v.visited_at = t.visited_at AND v.created_at = t.created_at AND v.id < t.id)
		WHERE t.id = $1
		GROUP BY t.id`, id).Scan(&position)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("visit position: %w", err)
	}
	return position, true, nil
}

func (s *Store) VisitPhoto(ctx context.Context, id uuid.UUID) (*model.VisitPhoto, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, visit_id, data_uri, content_type, byte_count, sort_order, created_at
		FROM visit_photos
		WHERE id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("visit photo: %w", err)
	}
	defer rows.Close()
	photos, err := scanVisitPhotos(rows)
	if err != nil {
		return nil, fmt.Errorf("scan visit photo: %w", err)
	}
	if len(photos) == 0 {
		return nil, nil
	}
	return &photos[0], nil
}

func (s *Store) RestaurantVisits(ctx context.Context, restaurantID uuid.UUID) ([]model.Visit, error) {
	rows, err := s.pool.Query(ctx, visitSelectSQL()+` WHERE v.restaurant_id = $1 ORDER BY v.visited_at DESC, v.created_at DESC`, restaurantID)
	if err != nil {
		return nil, fmt.Errorf("list restaurant visits: %w", err)
	}
	defer rows.Close()
	return s.scanVisits(ctx, rows, withPhotoMetadata)
}

// RestaurantVisitSummaries loads photo-free visits for several restaurants at once, keyed by restaurant ID.
func (s *Store) RestaurantVisitSummaries(ctx context.Context, restaurantIDs []uuid.UUID) (map[uuid.UUID][]model.Visit, error) {
	summaries := map[uuid.UUID][]model.Visit{}
	if len(restaurantIDs) == 0 {
		return summaries, nil
	}
	rows, err := s.pool.Query(ctx, visitSelectSQL()+` WHERE v.restaurant_id = ANY($1) ORDER BY v.visited_at DESC, v.created_at DESC`, restaurantIDs)
	if err != nil {
		return nil, fmt.Errorf("list restaurant visit summaries: %w", err)
	}
	defer rows.Close()
	visits, err := s.scanVisits(ctx, rows, withoutPhotos)
	if err != nil {
		return nil, err
	}
	for _, visit := range visits {
		summaries[visit.Restaurant.ID] = append(summaries[visit.Restaurant.ID], visit)
	}
	return summaries, nil
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

func (s *Store) CreateVisit(ctx context.Context, input model.VisitInput) (*uuid.UUID, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin create visit: %w", err)
	}
	defer tx.Rollback(ctx)

	latitude, longitude, phone, website, googleRating, googlePriceLevel := googleMetadataValues(input.GoogleMetadata)
	existingRestaurantID := input.RestaurantID
	restaurantID := input.RestaurantID
	if restaurantID == nil {
		var id uuid.UUID
		name := strings.TrimSpace(input.RestaurantName)
		address := strings.TrimSpace(input.Address)
		addressValue := nullableString(input.Address)
		city := nullableString(input.City)
		placeID := nullableString(input.GooglePlaceID)
		category := nullableString(input.Category)
		if placeID != nil {
			err := tx.QueryRow(ctx, `
					SELECT id
					FROM restaurants
					WHERE google_place_id = $1
					LIMIT 1`, *placeID).Scan(&id)
			if err == nil {
				restaurantID = &id
				_, err = tx.Exec(ctx, `
					UPDATE restaurants
					SET address = COALESCE(restaurants.address, $2),
					    city = COALESCE(restaurants.city, $3),
					    category = COALESCE(restaurants.category, $4),
					    latitude = COALESCE(restaurants.latitude, $5),
					    longitude = COALESCE(restaurants.longitude, $6),
					    phone = COALESCE(restaurants.phone, $7),
					    website = COALESCE(restaurants.website, $8),
					    google_rating = COALESCE(restaurants.google_rating, $9),
					    google_price_level = COALESCE(restaurants.google_price_level, $10),
					    updated_at = NOW()
					WHERE id = $1`, id, addressValue, city, category, latitude, longitude, phone, website, googleRating, googlePriceLevel)
				if err != nil {
					return nil, fmt.Errorf("update matched restaurant metadata: %w", err)
				}
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("find restaurant by place id: %w", err)
			}
		}
		if restaurantID == nil && address != "" {
			err := tx.QueryRow(ctx, `
					SELECT id
					FROM restaurants
					WHERE lower(name) = lower($1)
					  AND lower(coalesce(address, '')) = lower($2)
					ORDER BY created_at
					LIMIT 1`, name, address).Scan(&id)
			if err == nil {
				restaurantID = &id
				_, err = tx.Exec(ctx, `
					UPDATE restaurants
					SET google_place_id = COALESCE(restaurants.google_place_id, $2),
					    category = COALESCE(restaurants.category, $3),
					    city = COALESCE(restaurants.city, $4),
					    latitude = COALESCE(restaurants.latitude, $5),
					    longitude = COALESCE(restaurants.longitude, $6),
					    phone = COALESCE(restaurants.phone, $7),
					    website = COALESCE(restaurants.website, $8),
					    google_rating = COALESCE(restaurants.google_rating, $9),
					    google_price_level = COALESCE(restaurants.google_price_level, $10),
					    updated_at = NOW()
					WHERE id = $1`, id, placeID, category, city, latitude, longitude, phone, website, googleRating, googlePriceLevel)
				if err != nil {
					return nil, fmt.Errorf("update matched restaurant metadata: %w", err)
				}
			} else if !errors.Is(err, pgx.ErrNoRows) {
				return nil, fmt.Errorf("find restaurant by name and address: %w", err)
			}
		}
	}
	if restaurantID == nil {
		var id uuid.UUID
		name := strings.TrimSpace(input.RestaurantName)
		address := nullableString(input.Address)
		city := nullableString(input.City)
		placeID := nullableString(input.GooglePlaceID)
		category := nullableString(input.Category)
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
			RETURNING id`, name, address, city, latitude, longitude, phone, website, placeID, googleRating, googlePriceLevel, category, input.IsChain).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("create restaurant: %w", err)
		}
		restaurantID = &id
	}

	if existingRestaurantID != nil {
		_, err = tx.Exec(ctx, `
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
			*existingRestaurantID,
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
			return nil, fmt.Errorf("update existing restaurant metadata: %w", err)
		}
	}

	var visitID uuid.UUID
	notes := nullableString(input.Notes)
	err = tx.QueryRow(ctx, `
		INSERT INTO dining_visits (restaurant_id, visited_at, picked_by_person_id, price_level, notes)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`, *restaurantID, input.VisitedAt, input.PickerID, input.PriceLevel, notes).Scan(&visitID)
	if err != nil {
		return nil, fmt.Errorf("create visit: %w", err)
	}

	for personID, score := range input.Ratings {
		_, err := tx.Exec(ctx, `
			INSERT INTO visit_participant_ratings (visit_id, person_id, rating)
			VALUES ($1, $2, $3)`, visitID, personID, score)
		if err != nil {
			return nil, fmt.Errorf("create rating: %w", err)
		}
	}

	tagIDs := input.TagIDs
	if name := strings.TrimSpace(input.NewTag); name != "" {
		var tagID uuid.UUID
		err := tx.QueryRow(ctx, `
			INSERT INTO tags (name)
			VALUES ($1)
			ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id`, name).Scan(&tagID)
		if err != nil {
			return nil, fmt.Errorf("create tag: %w", err)
		}
		tagIDs = append(tagIDs, tagID)
	}
	for _, tagID := range tagIDs {
		_, err := tx.Exec(ctx, `
			INSERT INTO visit_tags (visit_id, tag_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, visitID, tagID)
		if err != nil {
			return nil, fmt.Errorf("create visit tag: %w", err)
		}
	}

	if err := insertVisitPhotos(ctx, tx, visitID, input.Photos, 0); err != nil {
		return nil, fmt.Errorf("create visit photos: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create visit: %w", err)
	}
	return &visitID, nil
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

func (s *Store) UpdateVisit(ctx context.Context, id uuid.UUID, input model.VisitInput) error {
	if err := input.Validate(); err != nil {
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

	if _, err := tx.Exec(ctx, `DELETE FROM visit_participant_ratings WHERE visit_id = $1`, id); err != nil {
		return fmt.Errorf("clear visit ratings: %w", err)
	}
	for personID, score := range input.Ratings {
		_, err := tx.Exec(ctx, `
			INSERT INTO visit_participant_ratings (visit_id, person_id, rating)
			VALUES ($1, $2, $3)`, id, personID, score)
		if err != nil {
			return fmt.Errorf("update rating: %w", err)
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM visit_tags WHERE visit_id = $1`, id); err != nil {
		return fmt.Errorf("clear visit tags: %w", err)
	}
	for _, tagID := range tagIDs {
		_, err := tx.Exec(ctx, `
			INSERT INTO visit_tags (visit_id, tag_id)
			VALUES ($1, $2)
			ON CONFLICT DO NOTHING`, id, tagID)
		if err != nil {
			return fmt.Errorf("update visit tag: %w", err)
		}
	}

	if err := reconcileVisitPhotos(ctx, tx, id, input); err != nil {
		return fmt.Errorf("update visit photos: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit update visit: %w", err)
	}
	return nil
}

func (s *Store) DeleteVisit(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM dining_visits WHERE id = $1`, id)
	return err
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
		    is_chain = $10
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

func (s *Store) Stats(ctx context.Context) (model.Stats, error) {
	var stats model.Stats
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM dining_visits`).Scan(&stats.TotalDines); err != nil {
		return stats, fmt.Errorf("count dines: %w", err)
	}
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(AVG(rating), 0) FROM visit_participant_ratings`).Scan(&stats.AverageRating); err != nil {
		return stats, fmt.Errorf("average rating: %w", err)
	}
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT restaurant_id)
		FROM dining_visits`).Scan(&stats.NewPlaces); err != nil {
		return stats, fmt.Errorf("count new places: %w", err)
	}
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT lower(btrim(r.city)))
		FROM restaurants r
		JOIN dining_visits v ON v.restaurant_id = r.id
		WHERE r.city IS NOT NULL AND btrim(r.city) <> ''`).Scan(&stats.CitiesExplored); err != nil {
		return stats, fmt.Errorf("count cities explored: %w", err)
	}
	err := s.pool.QueryRow(ctx, `
		SELECT r.name FROM restaurants r
		JOIN dining_visits v ON v.restaurant_id = r.id
		GROUP BY r.id, r.name
		ORDER BY COUNT(*) DESC, r.name
		LIMIT 1`).Scan(&stats.MostVisitedRestaurant)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return stats, fmt.Errorf("most visited restaurant: %w", err)
	}
	err = s.pool.QueryRow(ctx, `
		SELECT r.name FROM restaurants r
		JOIN dining_visits v ON v.restaurant_id = r.id
		JOIN visit_participant_ratings vr ON vr.visit_id = v.id
		GROUP BY r.id, r.name
		ORDER BY AVG(vr.rating) DESC, r.name
		LIMIT 1`).Scan(&stats.HighestRatedRestaurant)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return stats, fmt.Errorf("highest rated restaurant: %w", err)
	}
	err = s.pool.QueryRow(ctx, `
		SELECT p.name, AVG(vr.rating) FROM persons p
		JOIN dining_visits v ON v.picked_by_person_id = p.id
		JOIN visit_participant_ratings vr ON vr.visit_id = v.id
		GROUP BY p.id, p.name
		ORDER BY AVG(vr.rating) DESC, p.name
		LIMIT 1`).Scan(&stats.BestPicker, &stats.BestPickerAverage)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return stats, fmt.Errorf("best picker: %w", err)
	}
	err = s.pool.QueryRow(ctx, `
		SELECT p.name, AVG(vr.rating) FROM persons p
		JOIN dining_visits v ON v.picked_by_person_id = p.id
		JOIN visit_participant_ratings vr ON vr.visit_id = v.id
		GROUP BY p.id, p.name
		ORDER BY AVG(vr.rating) ASC, p.name
		LIMIT 1`).Scan(&stats.WorstPicker, &stats.WorstPickerAverage)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return stats, fmt.Errorf("worst picker: %w", err)
	}
	err = s.pool.QueryRow(ctx, `
		SELECT r.name FROM restaurants r
		JOIN dining_visits v ON v.restaurant_id = r.id
		JOIN visit_participant_ratings vr ON vr.visit_id = v.id
		GROUP BY v.id, r.name
		ORDER BY (MAX(vr.rating) - MIN(vr.rating)) DESC, r.name
		LIMIT 1`).Scan(&stats.BiggestSplitRestaurant)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return stats, fmt.Errorf("biggest split restaurant: %w", err)
	}
	stats.TopRestaurants, err = s.topRestaurants(ctx)
	if err != nil {
		return stats, err
	}
	stats.TopRestaurantsByCuisine, err = s.topRestaurantsByCuisine(ctx)
	if err != nil {
		return stats, err
	}
	return stats, nil
}

func (s *Store) topRestaurants(ctx context.Context) ([]model.RestaurantRatingStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.name, AVG(vr.rating), COUNT(vr.rating), COUNT(DISTINCT v.id)
		FROM restaurants r
		JOIN dining_visits v ON v.restaurant_id = r.id
		JOIN visit_participant_ratings vr ON vr.visit_id = v.id
		GROUP BY r.id, r.name
		HAVING COUNT(vr.rating) >= 2
		ORDER BY AVG(vr.rating) DESC, COUNT(vr.rating) DESC, r.name
		LIMIT 5`)
	if err != nil {
		return nil, fmt.Errorf("top restaurants: %w", err)
	}
	defer rows.Close()

	var restaurants []model.RestaurantRatingStat
	for rows.Next() {
		var restaurant model.RestaurantRatingStat
		if err := rows.Scan(&restaurant.Name, &restaurant.AverageRating, &restaurant.RatingCount, &restaurant.VisitCount); err != nil {
			return nil, fmt.Errorf("scan top restaurant: %w", err)
		}
		restaurants = append(restaurants, restaurant)
	}
	return restaurants, rows.Err()
}

func (s *Store) topRestaurantsByCuisine(ctx context.Context) ([]model.CuisineRestaurantStat, error) {
	rows, err := s.pool.Query(ctx, `
		WITH rated_restaurants AS (
			SELECT btrim(r.category) AS cuisine,
			       r.name,
			       AVG(vr.rating) AS average_rating,
			       COUNT(vr.rating) AS rating_count,
			       COUNT(DISTINCT v.id) AS visit_count
			FROM restaurants r
			JOIN dining_visits v ON v.restaurant_id = r.id
			JOIN visit_participant_ratings vr ON vr.visit_id = v.id
			WHERE r.category IS NOT NULL AND btrim(r.category) <> ''
			GROUP BY lower(btrim(r.category)), btrim(r.category), r.id, r.name
			HAVING COUNT(vr.rating) >= 1
		),
		ranked_restaurants AS (
			SELECT cuisine,
			       name,
			       average_rating,
			       rating_count,
			       visit_count,
			       ROW_NUMBER() OVER (
			           PARTITION BY lower(cuisine)
			           ORDER BY average_rating DESC, rating_count DESC, name
			       ) AS cuisine_rank
			FROM rated_restaurants
		)
		SELECT cuisine, name, average_rating, rating_count, visit_count
		FROM ranked_restaurants
		WHERE cuisine_rank = 1
		ORDER BY cuisine`)
	if err != nil {
		return nil, fmt.Errorf("top restaurants by cuisine: %w", err)
	}
	defer rows.Close()

	var restaurants []model.CuisineRestaurantStat
	for rows.Next() {
		var restaurant model.CuisineRestaurantStat
		if err := rows.Scan(&restaurant.Cuisine, &restaurant.Name, &restaurant.AverageRating, &restaurant.RatingCount, &restaurant.VisitCount); err != nil {
			return nil, fmt.Errorf("scan top restaurant by cuisine: %w", err)
		}
		restaurants = append(restaurants, restaurant)
	}
	return restaurants, rows.Err()
}

func (s *Store) PickerTurn(ctx context.Context) (model.PickerTurn, error) {
	people, err := s.People(ctx)
	if err != nil {
		return model.PickerTurn{}, err
	}
	if len(people) == 0 {
		return model.PickerTurn{}, nil
	}

	var last model.Person
	err = s.pool.QueryRow(ctx, `
		SELECT p.id, p.name, p.avatar_color
		FROM dining_visits v
		JOIN persons p ON p.id = v.picked_by_person_id
		ORDER BY v.visited_at DESC, v.created_at DESC
		LIMIT 1`).Scan(&last.ID, &last.Name, &last.AvatarColor)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.PickerTurn{NextPicker: people[0]}, nil
		}
		return model.PickerTurn{}, fmt.Errorf("latest picker: %w", err)
	}

	next := people[0]
	for i, person := range people {
		if person.ID == last.ID {
			next = people[(i+1)%len(people)]
			break
		}
	}
	return model.PickerTurn{LastPicker: last, NextPicker: next}, nil
}

// photoDetail controls how much photo data scanVisits loads alongside each visit.
type photoDetail int

const (
	withoutPhotos photoDetail = iota
	withPhotoMetadata
	withPhotoData
)

func (s *Store) scanVisits(ctx context.Context, rows pgx.Rows, photos photoDetail) ([]model.Visit, error) {
	defer rows.Close()
	var visits []model.Visit
	for rows.Next() {
		var visit model.Visit
		err := rows.Scan(
			&visit.ID,
			&visit.VisitedAt,
			&visit.PriceLevel,
			&visit.Notes,
			&visit.CreatedAt,
			&visit.UpdatedAt,
			&visit.Picker.ID,
			&visit.Picker.Name,
			&visit.Picker.AvatarColor,
			&visit.Restaurant.ID,
			&visit.Restaurant.Name,
			&visit.Restaurant.Address,
			&visit.Restaurant.City,
			&visit.Restaurant.Latitude,
			&visit.Restaurant.Longitude,
			&visit.Restaurant.Phone,
			&visit.Restaurant.Website,
			&visit.Restaurant.GooglePlaceID,
			&visit.Restaurant.GoogleRating,
			&visit.Restaurant.GooglePriceLevel,
			&visit.Restaurant.Category,
			&visit.Restaurant.IsChain,
			&visit.Restaurant.CreatedAt,
			&visit.Restaurant.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan visit: %w", err)
		}
		visits = append(visits, visit)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read visits: %w", err)
	}
	// Release the base query connection before loading related records from the pool.
	rows.Close()

	if err := s.loadVisitRelations(ctx, visits, photos); err != nil {
		return nil, err
	}
	return visits, nil
}

// loadVisitRelations fills ratings, tags, and optionally photos with one query per relation.
func (s *Store) loadVisitRelations(ctx context.Context, visits []model.Visit, photos photoDetail) error {
	if len(visits) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(visits))
	byID := make(map[uuid.UUID]*model.Visit, len(visits))
	for i := range visits {
		ids[i] = visits[i].ID
		byID[visits[i].ID] = &visits[i]
	}

	if err := s.loadVisitRatings(ctx, ids, byID); err != nil {
		return err
	}
	if err := s.loadVisitTags(ctx, ids, byID); err != nil {
		return err
	}
	if photos != withoutPhotos {
		if err := s.loadVisitPhotos(ctx, ids, byID, photos == withPhotoData); err != nil {
			return err
		}
	}
	return nil
}

func insertVisitPhotos(ctx context.Context, tx pgx.Tx, visitID uuid.UUID, photos []model.VisitPhotoInput, startOrder int) error {
	for i, photo := range photos {
		byteCount, err := model.ValidateVisitPhotoDataURI(photo.DataURI)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO visit_photos (visit_id, data_uri, content_type, byte_count, sort_order)
			VALUES ($1, $2, $3, $4, $5)`,
			visitID,
			strings.TrimSpace(photo.DataURI),
			"image/jpeg",
			byteCount,
			startOrder+i,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func reconcileVisitPhotos(ctx context.Context, tx pgx.Tx, visitID uuid.UUID, input model.VisitInput) error {
	existingPhotos, err := visitPhotosForTx(ctx, tx, visitID)
	if err != nil {
		return err
	}
	existingByID := map[uuid.UUID]model.VisitPhoto{}
	for _, photo := range existingPhotos {
		existingByID[photo.ID] = photo
	}

	if _, err := tx.Exec(ctx, `DELETE FROM visit_photos WHERE visit_id = $1`, visitID); err != nil {
		return err
	}
	for order, id := range input.KeepPhotoIDs {
		photo, ok := existingByID[id]
		if !ok {
			return errors.New("photo not found")
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO visit_photos (id, visit_id, data_uri, content_type, byte_count, sort_order, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			photo.ID,
			visitID,
			photo.DataURI,
			photo.ContentType,
			photo.ByteCount,
			order,
			photo.CreatedAt,
		)
		if err != nil {
			return err
		}
	}
	return insertVisitPhotos(ctx, tx, visitID, input.Photos, len(input.KeepPhotoIDs))
}

func visitPhotosForTx(ctx context.Context, tx pgx.Tx, visitID uuid.UUID) ([]model.VisitPhoto, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, visit_id, data_uri, content_type, byte_count, sort_order, created_at
		FROM visit_photos
		WHERE visit_id = $1
		ORDER BY sort_order, created_at, id`, visitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanVisitPhotos(rows)
}

func (s *Store) loadVisitRatings(ctx context.Context, ids []uuid.UUID, byID map[uuid.UUID]*model.Visit) error {
	rows, err := s.pool.Query(ctx, `
		SELECT r.visit_id, p.id, p.name, p.avatar_color, r.rating
		FROM visit_participant_ratings r
		JOIN persons p ON p.id = r.person_id
		WHERE r.visit_id = ANY($1)
		ORDER BY p.sort_order`, ids)
	if err != nil {
		return fmt.Errorf("list visit ratings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var visitID uuid.UUID
		var rating model.Rating
		if err := rows.Scan(&visitID, &rating.Person.ID, &rating.Person.Name, &rating.Person.AvatarColor, &rating.Score); err != nil {
			return fmt.Errorf("scan visit rating: %w", err)
		}
		if visit := byID[visitID]; visit != nil {
			visit.Ratings = append(visit.Ratings, rating)
		}
	}
	return rows.Err()
}

func (s *Store) loadVisitTags(ctx context.Context, ids []uuid.UUID, byID map[uuid.UUID]*model.Visit) error {
	rows, err := s.pool.Query(ctx, `
		SELECT vt.visit_id, t.id, t.name
		FROM visit_tags vt
		JOIN tags t ON t.id = vt.tag_id
		WHERE vt.visit_id = ANY($1)
		ORDER BY t.name`, ids)
	if err != nil {
		return fmt.Errorf("list visit tags: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var visitID uuid.UUID
		var tag model.Tag
		if err := rows.Scan(&visitID, &tag.ID, &tag.Name); err != nil {
			return fmt.Errorf("scan visit tag: %w", err)
		}
		if visit := byID[visitID]; visit != nil {
			visit.Tags = append(visit.Tags, tag)
		}
	}
	return rows.Err()
}

// loadVisitPhotos attaches photo metadata, plus the stored data URI only when includeData is set.
func (s *Store) loadVisitPhotos(ctx context.Context, ids []uuid.UUID, byID map[uuid.UUID]*model.Visit, includeData bool) error {
	dataColumn := "''"
	if includeData {
		dataColumn = "data_uri"
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, visit_id, `+dataColumn+`, content_type, byte_count, sort_order, created_at
		FROM visit_photos
		WHERE visit_id = ANY($1)
		ORDER BY sort_order, created_at, id`, ids)
	if err != nil {
		return fmt.Errorf("list visit photos: %w", err)
	}
	defer rows.Close()
	photos, err := scanVisitPhotos(rows)
	if err != nil {
		return fmt.Errorf("scan visit photos: %w", err)
	}
	for _, photo := range photos {
		if visit := byID[photo.VisitID]; visit != nil {
			visit.Photos = append(visit.Photos, photo)
		}
	}
	return nil
}

func scanVisitPhotos(rows pgx.Rows) ([]model.VisitPhoto, error) {
	var photos []model.VisitPhoto
	for rows.Next() {
		var photo model.VisitPhoto
		if err := rows.Scan(
			&photo.ID,
			&photo.VisitID,
			&photo.DataURI,
			&photo.ContentType,
			&photo.ByteCount,
			&photo.SortOrder,
			&photo.CreatedAt,
		); err != nil {
			return nil, err
		}
		photos = append(photos, photo)
	}
	return photos, rows.Err()
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
		&restaurant.CreatedAt,
		&restaurant.UpdatedAt,
	)
	if err != nil {
		return model.Restaurant{}, fmt.Errorf("scan restaurant: %w", err)
	}
	return restaurant, nil
}

func visitSelectSQL() string {
	return `
		SELECT v.id, v.visited_at, v.price_level, v.notes, v.created_at, v.updated_at,
		       p.id, p.name, p.avatar_color,
		       r.id, r.name, r.address, r.city, r.latitude, r.longitude, r.phone, r.website,
		       r.google_place_id, r.google_rating, r.google_price_level, r.category,
		       r.is_chain, r.created_at, r.updated_at
		FROM dining_visits v
		JOIN persons p ON p.id = v.picked_by_person_id
		JOIN restaurants r ON r.id = v.restaurant_id`
}

func nullableString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func googleMetadataValues(metadata model.GoogleRestaurantMetadata) (*float64, *float64, *string, *string, *float64, *int) {
	return metadata.Latitude,
		metadata.Longitude,
		nullableString(metadata.Phone),
		nullableString(metadata.Website),
		metadata.GoogleRating,
		metadata.GooglePriceLevel
}

var _ = time.Time{}
