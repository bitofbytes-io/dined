package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
)

// RestedFavorites lists the restaurants whose ratings average at least
// minAverage, counting only visits whose ratings count in aggregates, and
// whose latest visit, rated or not, was before lastVisitedBefore. They are
// ordered by name.
func (s *Store) RestedFavorites(ctx context.Context, minAverage float64, lastVisitedBefore time.Time) ([]model.RestedFavorite, error) {
	rows, err := s.pool.Query(ctx, `WITH`+aggregateRatingsCTE+`,
		favorites AS (
			SELECT v.restaurant_id, AVG(vr.rating) AS average_rating
			FROM dining_visits v
			JOIN aggregate_ratings vr ON vr.visit_id = v.id
			GROUP BY v.restaurant_id
			HAVING AVG(vr.rating) >= $2
		),
		last_visits AS (
			SELECT restaurant_id, MAX(visited_at) AS last_visited_at
			FROM dining_visits
			GROUP BY restaurant_id
		)
		SELECT r.id, r.name, r.address, r.city, r.latitude, r.longitude, r.phone, r.website,
		       r.google_place_id, r.google_rating, r.google_price_level, r.category,
		       r.is_chain, r.created_at, r.updated_at,
		       f.average_rating, l.last_visited_at
		FROM favorites f
		JOIN restaurants r ON r.id = f.restaurant_id
		JOIN last_visits l ON l.restaurant_id = r.id
		WHERE l.last_visited_at < $3
		ORDER BY r.name, r.id`, model.MinAggregateRaters, minAverage, lastVisitedBefore)
	if err != nil {
		return nil, fmt.Errorf("rested favorites: %w", err)
	}
	defer rows.Close()

	var favorites []model.RestedFavorite
	for rows.Next() {
		var favorite model.RestedFavorite
		restaurant := &favorite.Restaurant
		if err := rows.Scan(
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
			&favorite.AverageRating,
			&favorite.LastVisitedAt,
		); err != nil {
			return nil, fmt.Errorf("scan rested favorite: %w", err)
		}
		favorites = append(favorites, favorite)
	}
	return favorites, rows.Err()
}

// PickCounts counts each person's picks since a time, in people order, with
// their latest pick ever. Everybody picks count for no one.
func (s *Store) PickCounts(ctx context.Context, since time.Time) ([]model.PickCount, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.name, p.avatar_color,
		       COUNT(v.id) FILTER (WHERE v.visited_at >= $1),
		       MAX(v.visited_at)
		FROM persons p
		LEFT JOIN dining_visits v ON v.picked_by_person_id = p.id
		GROUP BY p.id, p.name, p.avatar_color, p.sort_order
		ORDER BY p.sort_order`, since)
	if err != nil {
		return nil, fmt.Errorf("pick counts: %w", err)
	}
	defer rows.Close()

	var counts []model.PickCount
	for rows.Next() {
		var count model.PickCount
		if err := rows.Scan(&count.Person.ID, &count.Person.Name, &count.Person.AvatarColor, &count.Picks, &count.LastPickedAt); err != nil {
			return nil, fmt.Errorf("scan pick count: %w", err)
		}
		counts = append(counts, count)
	}
	return counts, rows.Err()
}
