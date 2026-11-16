package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/jackc/pgx/v5"
)

// YearInReview summarizes the Eastern calendar year's dines: visits by month,
// new and repeat restaurants, the most visited and highest rated restaurants,
// and each person's best-rated pick.
func (s *Store) YearInReview(ctx context.Context, year int) (model.YearInReview, error) {
	review := model.YearInReview{Year: year}
	start, end := apptime.YearBounds(year)

	rows, err := s.pool.Query(ctx, `
		SELECT EXTRACT(MONTH FROM visited_at AT TIME ZONE $3::text)::int, COUNT(*)
		FROM dining_visits
		WHERE visited_at >= $1 AND visited_at < $2
		GROUP BY 1`, start, end, apptime.EasternLocation().String())
	if err != nil {
		return review, fmt.Errorf("year visits by month: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var month, count int
		if err := rows.Scan(&month, &count); err != nil {
			return review, fmt.Errorf("scan year visits by month: %w", err)
		}
		review.MonthlyVisits[month-1] = count
		review.TotalVisits += count
	}
	if err := rows.Err(); err != nil {
		return review, fmt.Errorf("year visits by month: %w", err)
	}
	rows.Close()

	// A restaurant is new when its first visit ever falls in the year.
	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE first_visit >= $1),
		       COUNT(*) FILTER (WHERE first_visit < $1)
		FROM (
			SELECT MIN(visited_at) AS first_visit
			FROM dining_visits
			GROUP BY restaurant_id
			HAVING COUNT(*) FILTER (WHERE visited_at >= $1 AND visited_at < $2) > 0
		) year_restaurants`, start, end).Scan(&review.NewRestaurants, &review.RepeatRestaurants); err != nil {
		return review, fmt.Errorf("year new and repeat restaurants: %w", err)
	}

	var mostVisited model.YearRestaurantStat
	err = s.pool.QueryRow(ctx, `
		SELECT r.id, r.name, COUNT(*)
		FROM dining_visits v
		JOIN restaurants r ON r.id = v.restaurant_id
		WHERE v.visited_at >= $1 AND v.visited_at < $2
		GROUP BY r.id, r.name
		HAVING COUNT(*) >= 2
		ORDER BY COUNT(*) DESC, MAX(v.visited_at) DESC, r.name, r.id
		LIMIT 1`, start, end).Scan(&mostVisited.ID, &mostVisited.Name, &mostVisited.VisitCount)
	switch {
	case err == nil:
		review.MostVisited = &mostVisited
	case !errors.Is(err, pgx.ErrNoRows):
		return review, fmt.Errorf("year most visited restaurant: %w", err)
	}

	var highestRated model.YearRestaurantStat
	err = s.pool.QueryRow(ctx, `WITH`+aggregateRatingsCTE+`
		SELECT r.id, r.name, COUNT(DISTINCT v.id), AVG(vr.rating)
		FROM restaurants r
		JOIN dining_visits v ON v.restaurant_id = r.id
		JOIN aggregate_ratings vr ON vr.visit_id = v.id
		WHERE v.visited_at >= $2 AND v.visited_at < $3
		GROUP BY r.id, r.name
		ORDER BY AVG(vr.rating) DESC, COUNT(vr.rating) DESC, r.name, r.id
		LIMIT 1`, model.MinAggregateRaters, start, end).Scan(&highestRated.ID, &highestRated.Name, &highestRated.VisitCount, &highestRated.AverageRating)
	switch {
	case err == nil:
		review.HighestRated = &highestRated
	case !errors.Is(err, pgx.ErrNoRows):
		return review, fmt.Errorf("year highest rated restaurant: %w", err)
	}

	// Everybody visits have no picker, so the join to persons leaves them out.
	pickRows, err := s.pool.Query(ctx, `WITH`+aggregateRatingsCTE+`,
		person_picks AS (
			SELECT p.id AS person_id,
			       p.name AS person,
			       p.sort_order,
			       r.id AS restaurant_id,
			       r.name AS restaurant,
			       AVG(vr.rating) AS average_rating,
			       COUNT(vr.rating) AS rating_count
			FROM persons p
			JOIN dining_visits v ON v.picked_by_person_id = p.id
			JOIN restaurants r ON r.id = v.restaurant_id
			JOIN aggregate_ratings vr ON vr.visit_id = v.id
			WHERE v.visited_at >= $2 AND v.visited_at < $3
			GROUP BY p.id, p.name, p.sort_order, r.id, r.name
		),
		ranked_picks AS (
			SELECT *,
			       ROW_NUMBER() OVER (
			           PARTITION BY person_id
			           ORDER BY average_rating DESC, rating_count DESC, restaurant, restaurant_id
			       ) AS pick_rank
			FROM person_picks
		)
		SELECT person, restaurant_id, restaurant, average_rating
		FROM ranked_picks
		WHERE pick_rank = 1
		ORDER BY sort_order`, model.MinAggregateRaters, start, end)
	if err != nil {
		return review, fmt.Errorf("year top picks: %w", err)
	}
	defer pickRows.Close()
	for pickRows.Next() {
		var pick model.PersonTopPick
		if err := pickRows.Scan(&pick.Person, &pick.RestaurantID, &pick.Restaurant, &pick.AverageRating); err != nil {
			return review, fmt.Errorf("scan year top pick: %w", err)
		}
		review.TopPicks = append(review.TopPicks, pick)
	}
	return review, pickRows.Err()
}
