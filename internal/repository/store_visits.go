package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) Visit(ctx context.Context, id uuid.UUID) (*model.Visit, error) {
	rows, err := s.pool.Query(ctx, visitSelectSQL()+` WHERE v.id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("visit: %w", err)
	}
	defer rows.Close()

	visits, err := s.scanVisits(ctx, rows, withPhotoMetadata)
	if err != nil {
		return nil, err
	}
	if len(visits) == 0 {
		return nil, nil
	}
	return &visits[0], nil
}

func (s *Store) Visits(ctx context.Context, limit int) ([]model.Visit, error) {
	return s.VisitsPage(ctx, model.VisitFilter{}, limit, 0)
}

// VisitsPage lists the visits that match filter, newest first. A limit of zero returns every
// matching visit after offset. Photos carry metadata only; their image bytes are served by VisitPhoto.
func (s *Store) VisitsPage(ctx context.Context, filter model.VisitFilter, limit, offset int) ([]model.Visit, error) {
	where, args := visitFilterSQL(filter)
	args = append(args, max(offset, 0))
	query := visitSelectSQL() + where + fmt.Sprintf(` ORDER BY v.visited_at DESC, v.created_at DESC, v.id OFFSET $%d`, len(args))
	if limit > 0 {
		args = append(args, limit)
		query += fmt.Sprintf(` LIMIT $%d`, len(args))
	}
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list visits: %w", err)
	}
	defer rows.Close()
	return s.scanVisits(ctx, rows, withPhotoMetadata)
}

// CountVisits counts the visits that match filter.
func (s *Store) CountVisits(ctx context.Context, filter model.VisitFilter) (int, error) {
	where, args := visitFilterSQL(filter)
	var count int
	err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM dining_visits v
		JOIN restaurants r ON r.id = v.restaurant_id`+where, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count visits: %w", err)
	}
	return count, nil
}

// VisitFilterOptions lists the cuisines and cities of visited restaurants and the years of visits.
func (s *Store) VisitFilterOptions(ctx context.Context) (model.VisitFilterOptions, error) {
	var options model.VisitFilterOptions
	var err error
	if options.Cuisines, err = s.visitedRestaurantValues(ctx, "category"); err != nil {
		return options, err
	}
	if options.Cities, err = s.visitedRestaurantValues(ctx, "city"); err != nil {
		return options, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT EXTRACT(YEAR FROM visited_at AT TIME ZONE 'America/New_York')::int AS year
		FROM dining_visits
		ORDER BY year DESC`)
	if err != nil {
		return options, fmt.Errorf("list visit years: %w", err)
	}
	if options.Years, err = pgx.CollectRows(rows, pgx.RowTo[int]); err != nil {
		return options, fmt.Errorf("scan visit years: %w", err)
	}
	return options, nil
}

// visitedRestaurantValues lists one trimmed spelling of each case-insensitive value of a
// restaurant text column, across restaurants with a visit. Byte order picks the spelling and
// sorts the values, like the memory store. column must be a trusted name.
func (s *Store) visitedRestaurantValues(ctx context.Context, column string) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT MIN(btrim(r.`+column+`) COLLATE "C")
		FROM restaurants r
		WHERE btrim(coalesce(r.`+column+`, '')) <> ''
		  AND EXISTS (SELECT 1 FROM dining_visits v WHERE v.restaurant_id = r.id)
		GROUP BY lower(btrim(r.`+column+`))
		ORDER BY lower(btrim(r.`+column+`)) COLLATE "C"`)
	if err != nil {
		return nil, fmt.Errorf("list restaurant %s values: %w", column, err)
	}
	values, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("scan restaurant %s values: %w", column, err)
	}
	return values, nil
}

// visitFilterSQL returns the WHERE clause, if any, and its arguments for filter. The clause
// refers to the visit as v and its restaurant as r, and numbers its parameters from $1.
func visitFilterSQL(filter model.VisitFilter) (string, []any) {
	var conditions []string
	var args []any
	arg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}
	if cuisine := strings.TrimSpace(filter.Cuisine); cuisine != "" {
		conditions = append(conditions, `lower(btrim(r.category)) = lower(`+arg(cuisine)+`)`)
	}
	if filter.EverybodyPicked {
		conditions = append(conditions, `v.picked_by_person_id IS NULL`)
	} else if filter.PickerID != nil {
		conditions = append(conditions, `v.picked_by_person_id = `+arg(*filter.PickerID))
	}
	if filter.RaterID != nil {
		conditions = append(conditions, `EXISTS (SELECT 1 FROM visit_participant_ratings vr WHERE vr.visit_id = v.id AND vr.person_id = `+arg(*filter.RaterID)+`)`)
	}
	if filter.TagID != nil {
		conditions = append(conditions, `EXISTS (SELECT 1 FROM visit_tags vt WHERE vt.visit_id = v.id AND vt.tag_id = `+arg(*filter.TagID)+`)`)
	}
	if city := strings.TrimSpace(filter.City); city != "" {
		conditions = append(conditions, `lower(btrim(r.city)) = lower(`+arg(city)+`)`)
	}
	if filter.Year != 0 {
		start, end := apptime.YearBounds(filter.Year)
		conditions = append(conditions, `v.visited_at >= `+arg(start)+` AND v.visited_at < `+arg(end))
	}
	if len(conditions) == 0 {
		return "", nil
	}
	return ` WHERE ` + strings.Join(conditions, ` AND `), args
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

func (s *Store) DeleteVisit(ctx context.Context, id uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM dining_visits WHERE id = $1`, id)
	return err
}

// photoDetail controls how much photo data scanVisits loads alongside each visit.
type photoDetail int

const (
	withoutPhotos photoDetail = iota
	withPhotoMetadata
)

func (s *Store) scanVisits(ctx context.Context, rows pgx.Rows, photos photoDetail) ([]model.Visit, error) {
	defer rows.Close()
	var visits []model.Visit
	for rows.Next() {
		var visit model.Visit
		var pickerID *uuid.UUID
		var pickerName, pickerAvatarColor *string
		err := rows.Scan(
			&visit.ID,
			&visit.VisitedAt,
			&visit.PriceLevel,
			&visit.Notes,
			&visit.CreatedAt,
			&visit.UpdatedAt,
			&pickerID,
			&pickerName,
			&pickerAvatarColor,
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
			&visit.Restaurant.ClosedAt,
			&visit.Restaurant.CreatedAt,
			&visit.Restaurant.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan visit: %w", err)
		}
		if pickerID != nil {
			visit.Picker = &model.Person{ID: *pickerID, Name: valueOrEmpty(pickerName), AvatarColor: valueOrEmpty(pickerAvatarColor)}
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
		if err := s.loadVisitPhotos(ctx, ids, byID); err != nil {
			return err
		}
	}
	return nil
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

func visitSelectSQL() string {
	return `
		SELECT v.id, v.visited_at, v.price_level, v.notes, v.created_at, v.updated_at,
		       p.id, p.name, p.avatar_color,
		       r.id, r.name, r.address, r.city, r.latitude, r.longitude, r.phone, r.website,
		       r.google_place_id, r.google_rating, r.google_price_level, r.category,
		       r.is_chain, r.closed_at, r.created_at, r.updated_at
		FROM dining_visits v
		LEFT JOIN persons p ON p.id = v.picked_by_person_id
		JOIN restaurants r ON r.id = v.restaurant_id`
}
