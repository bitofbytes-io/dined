package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/jackc/pgx/v5"
)

// aggregateRatingsCTE names the ratings that count in averages and rankings
// across visits: those on visits that at least $1 people rated.
const aggregateRatingsCTE = `
	aggregate_ratings AS (
		SELECT visit_id, rating
		FROM visit_participant_ratings
		WHERE visit_id IN (
			SELECT visit_id
			FROM visit_participant_ratings
			GROUP BY visit_id
			HAVING COUNT(*) >= $1
		)
	)`

func (s *Store) Stats(ctx context.Context) (model.Stats, error) {
	var stats model.Stats
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM dining_visits`).Scan(&stats.TotalDines); err != nil {
		return stats, fmt.Errorf("count dines: %w", err)
	}
	if err := s.pool.QueryRow(ctx, `WITH`+aggregateRatingsCTE+`
		SELECT COALESCE(AVG(rating), 0) FROM aggregate_ratings`, model.MinAggregateRaters).Scan(&stats.AverageRating); err != nil {
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
	// Everybody visits have no picker, so the join to persons leaves them out.
	err := s.pool.QueryRow(ctx, `WITH`+aggregateRatingsCTE+`
		SELECT p.name, AVG(vr.rating) FROM persons p
		JOIN dining_visits v ON v.picked_by_person_id = p.id
		JOIN aggregate_ratings vr ON vr.visit_id = v.id
		GROUP BY p.id, p.name
		ORDER BY AVG(vr.rating) DESC, p.name
		LIMIT 1`, model.MinAggregateRaters).Scan(&stats.BestPicker, &stats.BestPickerAverage)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return stats, fmt.Errorf("best picker: %w", err)
	}
	err = s.pool.QueryRow(ctx, `WITH`+aggregateRatingsCTE+`
		SELECT p.name, AVG(vr.rating) FROM persons p
		JOIN dining_visits v ON v.picked_by_person_id = p.id
		JOIN aggregate_ratings vr ON vr.visit_id = v.id
		GROUP BY p.id, p.name
		ORDER BY AVG(vr.rating) ASC, p.name
		LIMIT 1`, model.MinAggregateRaters).Scan(&stats.WorstPicker, &stats.WorstPickerAverage)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return stats, fmt.Errorf("worst picker: %w", err)
	}
	stats.TopRestaurants, err = s.topRestaurants(ctx)
	if err != nil {
		return stats, err
	}
	stats.TopRestaurantsByCuisine, err = s.topRestaurantsByCuisine(ctx)
	if err != nil {
		return stats, err
	}
	// A split needs two different ratings, so single-rater visits never win.
	err = s.pool.QueryRow(ctx, `
		SELECT r.name, MAX(vr.rating) - MIN(vr.rating)
		FROM dining_visits v
		JOIN restaurants r ON r.id = v.restaurant_id
		JOIN visit_participant_ratings vr ON vr.visit_id = v.id
		GROUP BY v.id, r.name, v.visited_at
		HAVING MAX(vr.rating) > MIN(vr.rating)
		ORDER BY MAX(vr.rating) - MIN(vr.rating) DESC, v.visited_at DESC, r.name
		LIMIT 1`).Scan(&stats.BiggestSplitRestaurant, &stats.BiggestSplit)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return stats, fmt.Errorf("biggest rating split: %w", err)
	}
	err = s.pool.QueryRow(ctx, `
		SELECT r.name, COUNT(*)
		FROM restaurants r
		JOIN dining_visits v ON v.restaurant_id = r.id
		GROUP BY r.id, r.name
		HAVING COUNT(*) >= 2
		ORDER BY COUNT(*) DESC, MAX(v.visited_at) DESC, r.name
		LIMIT 1`).Scan(&stats.MostLoyalRestaurant, &stats.MostLoyalVisits)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return stats, fmt.Errorf("most loyal restaurant: %w", err)
	}
	// The restaurant visited most recently is never "haven't been since".
	err = s.pool.QueryRow(ctx, `
		SELECT r.name, MAX(v.visited_at)
		FROM restaurants r
		JOIN dining_visits v ON v.restaurant_id = r.id
		GROUP BY r.id, r.name
		HAVING MAX(v.visited_at) < (SELECT MAX(visited_at) FROM dining_visits)
		ORDER BY MAX(v.visited_at), r.name
		LIMIT 1`).Scan(&stats.NostalgiaRestaurant, &stats.NostalgiaLastVisitedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return stats, fmt.Errorf("nostalgia restaurant: %w", err)
	}
	stats.TasteProfiles, err = s.tasteProfiles(ctx)
	if err != nil {
		return stats, err
	}
	return stats, nil
}

// tasteProfiles returns each person's highest-rated category on average,
// counting only the visits whose ratings count in aggregates.
func (s *Store) tasteProfiles(ctx context.Context) ([]model.PersonTasteStat, error) {
	rows, err := s.pool.Query(ctx, `
		WITH counted_visits AS (
			SELECT visit_id
			FROM visit_participant_ratings
			GROUP BY visit_id
			HAVING COUNT(*) >= $1
		),
		person_categories AS (
			SELECT p.id AS person_id,
			       p.name,
			       p.sort_order,
			       MIN(btrim(r.category)) AS category,
			       AVG(vr.rating) AS average_rating,
			       COUNT(*) AS rating_count
			FROM visit_participant_ratings vr
			JOIN counted_visits cv ON cv.visit_id = vr.visit_id
			JOIN persons p ON p.id = vr.person_id
			JOIN dining_visits v ON v.id = vr.visit_id
			JOIN restaurants r ON r.id = v.restaurant_id
			WHERE r.category IS NOT NULL AND btrim(r.category) <> ''
			GROUP BY p.id, p.name, p.sort_order, lower(btrim(r.category))
		),
		ranked_categories AS (
			SELECT *,
			       ROW_NUMBER() OVER (
			           PARTITION BY person_id
			           ORDER BY average_rating DESC, rating_count DESC, category
			       ) AS category_rank
			FROM person_categories
		)
		SELECT name, category, average_rating, rating_count
		FROM ranked_categories
		WHERE category_rank = 1
		ORDER BY sort_order`, model.MinAggregateRaters)
	if err != nil {
		return nil, fmt.Errorf("taste profiles: %w", err)
	}
	defer rows.Close()

	var profiles []model.PersonTasteStat
	for rows.Next() {
		var profile model.PersonTasteStat
		if err := rows.Scan(&profile.Person, &profile.Category, &profile.AverageRating, &profile.RatingCount); err != nil {
			return nil, fmt.Errorf("scan taste profile: %w", err)
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func (s *Store) topRestaurants(ctx context.Context) ([]model.RestaurantRatingStat, error) {
	rows, err := s.pool.Query(ctx, `WITH`+aggregateRatingsCTE+`
		SELECT r.name, AVG(vr.rating), COUNT(vr.rating), COUNT(DISTINCT v.id)
		FROM restaurants r
		JOIN dining_visits v ON v.restaurant_id = r.id
		JOIN aggregate_ratings vr ON vr.visit_id = v.id
		GROUP BY r.id, r.name
		HAVING COUNT(vr.rating) >= 2
		ORDER BY AVG(vr.rating) DESC, COUNT(vr.rating) DESC, r.name
		LIMIT 5`, model.MinAggregateRaters)
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
	rows, err := s.pool.Query(ctx, `WITH`+aggregateRatingsCTE+`,
		rated_restaurants AS (
			SELECT btrim(r.category) AS cuisine,
			       r.name,
			       AVG(vr.rating) AS average_rating,
			       COUNT(vr.rating) AS rating_count,
			       COUNT(DISTINCT v.id) AS visit_count
			FROM restaurants r
			JOIN dining_visits v ON v.restaurant_id = r.id
			JOIN aggregate_ratings vr ON vr.visit_id = v.id
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
		ORDER BY cuisine`, model.MinAggregateRaters)
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
