package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const wantToTrySelectSQL = `
	SELECT w.id, w.note, w.created_at, w.done_at,
	       p.id, p.name, p.avatar_color,
	       r.id, r.name, r.address, r.city, r.latitude, r.longitude, r.phone, r.website,
	       r.google_place_id, r.google_rating, r.google_price_level, r.category,
	       r.is_chain, r.created_at, r.updated_at
	FROM want_to_try w
	JOIN restaurants r ON r.id = w.restaurant_id
	LEFT JOIN persons p ON p.id = w.suggested_by_person_id`

// WantToTry lists the places still on the Want to Try list, newest first.
func (s *Store) WantToTry(ctx context.Context) ([]model.WantToTryItem, error) {
	rows, err := s.pool.Query(ctx, wantToTrySelectSQL+`
		WHERE w.done_at IS NULL
		ORDER BY w.created_at DESC, w.id`)
	if err != nil {
		return nil, fmt.Errorf("list want to try: %w", err)
	}
	defer rows.Close()

	var items []model.WantToTryItem
	for rows.Next() {
		item, err := scanWantToTryItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// WantToTryItem returns one Want to Try item, including one a dine has taken
// off the list, or nil if it does not exist.
func (s *Store) WantToTryItem(ctx context.Context, id uuid.UUID) (*model.WantToTryItem, error) {
	item, err := scanWantToTryItem(s.pool.QueryRow(ctx, wantToTrySelectSQL+` WHERE w.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// SaveWantToTry puts a restaurant on the Want to Try list, finding or creating
// it the same way a new dine does. Saving a restaurant that is already listed
// replaces its suggester and note; saving one a dine took off the list puts it
// back on as a new item.
func (s *Store) SaveWantToTry(ctx context.Context, input model.WantToTryInput) (*uuid.UUID, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin save want to try: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := checkPicker(ctx, tx, input.SuggestedByID); err != nil {
		return nil, err
	}
	if input.RestaurantID != nil {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM restaurants WHERE id = $1)`, *input.RestaurantID).Scan(&exists); err != nil {
			return nil, fmt.Errorf("check want to try restaurant: %w", err)
		}
		if !exists {
			return nil, model.ErrRestaurantNotFound
		}
	}
	restaurantID, err := resolveRestaurant(ctx, tx, input.VisitInput())
	if err != nil {
		return nil, err
	}

	var id uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO want_to_try (restaurant_id, suggested_by_person_id, note)
		VALUES ($1, $2, $3)
		ON CONFLICT (restaurant_id) DO UPDATE
		SET suggested_by_person_id = EXCLUDED.suggested_by_person_id,
		    note = EXCLUDED.note,
		    created_at = CASE WHEN want_to_try.done_at IS NULL THEN want_to_try.created_at ELSE NOW() END,
		    done_at = NULL
		RETURNING id`, restaurantID, input.SuggestedByID, nullableString(input.Note)).Scan(&id)
	if err != nil {
		return nil, fmt.Errorf("save want to try: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit save want to try: %w", err)
	}
	return &id, nil
}

// DeleteWantToTry removes an item from the Want to Try list. The restaurant stays.
func (s *Store) DeleteWantToTry(ctx context.Context, id uuid.UUID) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM want_to_try WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete want to try: %w", err)
	}
	return nil
}

// markWantToTryDone takes a restaurant off the Want to Try list because a dine
// was logged there.
func markWantToTryDone(ctx context.Context, tx pgx.Tx, restaurantID uuid.UUID) error {
	_, err := tx.Exec(ctx, `
		UPDATE want_to_try
		SET done_at = NOW()
		WHERE restaurant_id = $1 AND done_at IS NULL`, restaurantID)
	if err != nil {
		return fmt.Errorf("mark want to try done: %w", err)
	}
	return nil
}

func scanWantToTryItem(row pgx.Row) (model.WantToTryItem, error) {
	var item model.WantToTryItem
	var personID *uuid.UUID
	var personName, personAvatarColor *string
	err := row.Scan(
		&item.ID,
		&item.Note,
		&item.CreatedAt,
		&item.DoneAt,
		&personID,
		&personName,
		&personAvatarColor,
		&item.Restaurant.ID,
		&item.Restaurant.Name,
		&item.Restaurant.Address,
		&item.Restaurant.City,
		&item.Restaurant.Latitude,
		&item.Restaurant.Longitude,
		&item.Restaurant.Phone,
		&item.Restaurant.Website,
		&item.Restaurant.GooglePlaceID,
		&item.Restaurant.GoogleRating,
		&item.Restaurant.GooglePriceLevel,
		&item.Restaurant.Category,
		&item.Restaurant.IsChain,
		&item.Restaurant.CreatedAt,
		&item.Restaurant.UpdatedAt,
	)
	if err != nil {
		return model.WantToTryItem{}, fmt.Errorf("scan want to try: %w", err)
	}
	if personID != nil {
		item.SuggestedBy = &model.Person{ID: *personID, Name: valueOrEmpty(personName), AvatarColor: valueOrEmpty(personAvatarColor)}
	}
	return item, nil
}
