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
	Visits(context.Context, int) ([]model.Visit, error)
	RestaurantVisits(context.Context, uuid.UUID) ([]model.Visit, error)
	CreateVisit(context.Context, model.VisitInput) (*uuid.UUID, error)
	DeleteVisit(context.Context, uuid.UUID) error
	ToggleChain(context.Context, uuid.UUID, bool) error
	Stats(context.Context) (model.Stats, error)
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
		SELECT id, name, address, latitude, longitude, phone, website, google_place_id,
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
		SELECT id, name, address, latitude, longitude, phone, website, google_place_id,
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

func (s *Store) Visits(ctx context.Context, limit int) ([]model.Visit, error) {
	query := visitSelectSQL() + ` ORDER BY v.visited_at DESC`
	args := []any{}
	if limit > 0 {
		query += ` LIMIT $1`
		args = append(args, limit)
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list visits: %w", err)
	}
	defer rows.Close()
	return s.scanVisits(ctx, rows)
}

func (s *Store) RestaurantVisits(ctx context.Context, restaurantID uuid.UUID) ([]model.Visit, error) {
	rows, err := s.pool.Query(ctx, visitSelectSQL()+` WHERE v.restaurant_id = $1 ORDER BY v.visited_at DESC`, restaurantID)
	if err != nil {
		return nil, fmt.Errorf("list restaurant visits: %w", err)
	}
	defer rows.Close()
	return s.scanVisits(ctx, rows)
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

	restaurantID := input.RestaurantID
	if restaurantID == nil {
		var id uuid.UUID
		name := strings.TrimSpace(input.RestaurantName)
		address := strings.TrimSpace(input.Address)
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
					    updated_at = NOW()
					WHERE id = $1`, id, placeID, category)
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
		placeID := nullableString(input.GooglePlaceID)
		category := nullableString(input.Category)
		err := tx.QueryRow(ctx, `
				INSERT INTO restaurants (name, address, google_place_id, category, is_chain)
				VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (google_place_id) WHERE google_place_id IS NOT NULL DO UPDATE
			SET name = EXCLUDED.name,
			    address = COALESCE(EXCLUDED.address, restaurants.address),
			    category = COALESCE(EXCLUDED.category, restaurants.category),
			    updated_at = NOW()
			RETURNING id`, name, address, placeID, category, input.IsChain).Scan(&id)
		if err != nil {
			return nil, fmt.Errorf("create restaurant: %w", err)
		}
		restaurantID = &id
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
		if score == 0 {
			continue
		}
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

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit create visit: %w", err)
	}
	return &visitID, nil
}

func (s *Store) DeleteVisit(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM dining_visits WHERE id = $1`, id)
	return err
}

func (s *Store) ToggleChain(ctx context.Context, id uuid.UUID, isChain bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE restaurants SET is_chain = $2 WHERE id = $1`, id, isChain)
	return err
}

func (s *Store) Stats(ctx context.Context) (model.Stats, error) {
	var stats model.Stats
	_ = s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM dining_visits`).Scan(&stats.TotalDines)
	_ = s.pool.QueryRow(ctx, `SELECT COALESCE(AVG(rating), 0) FROM visit_participant_ratings`).Scan(&stats.AverageRating)
	_ = s.pool.QueryRow(ctx, `
		SELECT r.name FROM restaurants r
		JOIN dining_visits v ON v.restaurant_id = r.id
		GROUP BY r.id, r.name
		ORDER BY COUNT(*) DESC, r.name
		LIMIT 1`).Scan(&stats.MostVisitedRestaurant)
	_ = s.pool.QueryRow(ctx, `
		SELECT r.name FROM restaurants r
		JOIN dining_visits v ON v.restaurant_id = r.id
		JOIN visit_participant_ratings vr ON vr.visit_id = v.id
		GROUP BY r.id, r.name
		ORDER BY AVG(vr.rating) DESC, r.name
		LIMIT 1`).Scan(&stats.HighestRatedRestaurant)
	_ = s.pool.QueryRow(ctx, `
		SELECT p.name FROM persons p
		JOIN dining_visits v ON v.picked_by_person_id = p.id
		JOIN visit_participant_ratings vr ON vr.visit_id = v.id
		GROUP BY p.id, p.name
		ORDER BY AVG(vr.rating) DESC, p.name
		LIMIT 1`).Scan(&stats.BestPicker)
	_ = s.pool.QueryRow(ctx, `
		SELECT r.name FROM restaurants r
		JOIN dining_visits v ON v.restaurant_id = r.id
		JOIN visit_participant_ratings vr ON vr.visit_id = v.id
		GROUP BY v.id, r.name
		ORDER BY (MAX(vr.rating) - MIN(vr.rating)) DESC, r.name
		LIMIT 1`).Scan(&stats.BiggestSplitRestaurant)
	return stats, nil
}

func (s *Store) scanVisits(ctx context.Context, rows pgx.Rows) ([]model.Visit, error) {
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
		visit.Ratings, err = s.visitRatings(ctx, visit.ID)
		if err != nil {
			return nil, err
		}
		visit.Tags, err = s.visitTags(ctx, visit.ID)
		if err != nil {
			return nil, err
		}
		visits = append(visits, visit)
	}
	return visits, rows.Err()
}

func (s *Store) visitRatings(ctx context.Context, visitID uuid.UUID) ([]model.Rating, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.name, p.avatar_color, r.rating
		FROM visit_participant_ratings r
		JOIN persons p ON p.id = r.person_id
		WHERE r.visit_id = $1
		ORDER BY p.sort_order`, visitID)
	if err != nil {
		return nil, fmt.Errorf("list visit ratings: %w", err)
	}
	defer rows.Close()

	var ratings []model.Rating
	for rows.Next() {
		var rating model.Rating
		if err := rows.Scan(&rating.Person.ID, &rating.Person.Name, &rating.Person.AvatarColor, &rating.Score); err != nil {
			return nil, err
		}
		ratings = append(ratings, rating)
	}
	return ratings, rows.Err()
}

func (s *Store) visitTags(ctx context.Context, visitID uuid.UUID) ([]model.Tag, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.id, t.name
		FROM visit_tags vt
		JOIN tags t ON t.id = vt.tag_id
		WHERE vt.visit_id = $1
		ORDER BY t.name`, visitID)
	if err != nil {
		return nil, fmt.Errorf("list visit tags: %w", err)
	}
	defer rows.Close()

	var tags []model.Tag
	for rows.Next() {
		var tag model.Tag
		if err := rows.Scan(&tag.ID, &tag.Name); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
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
		       r.id, r.name, r.address, r.latitude, r.longitude, r.phone, r.website,
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

var _ = time.Time{}
