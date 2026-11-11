package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

type DinerStore interface {
	// Ping reports whether the backing database is reachable.
	Ping(context.Context) error
	People(context.Context) ([]model.Person, error)
	Tags(context.Context) ([]model.Tag, error)
	Restaurants(context.Context, string) ([]model.Restaurant, error)
	Restaurant(context.Context, uuid.UUID) (*model.Restaurant, error)
	RestaurantsByPlaceIDs(context.Context, []string) (map[string]model.Restaurant, error)
	Visit(context.Context, uuid.UUID) (*model.Visit, error)
	Visits(context.Context, int) ([]model.Visit, error)
	VisitsPage(ctx context.Context, filter model.VisitFilter, limit, offset int) ([]model.Visit, error)
	CountVisits(context.Context, model.VisitFilter) (int, error)
	VisitFilterOptions(context.Context) (model.VisitFilterOptions, error)
	VisitPosition(context.Context, uuid.UUID) (int, bool, error)
	VisitPhoto(context.Context, uuid.UUID, string) (*model.VisitPhoto, error)
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
	WantToTry(context.Context) ([]model.WantToTryItem, error)
	WantToTryItem(context.Context, uuid.UUID) (*model.WantToTryItem, error)
	SaveWantToTry(context.Context, model.WantToTryInput) (*uuid.UUID, error)
	DeleteWantToTry(context.Context, uuid.UUID) error
	RestedFavorites(ctx context.Context, minAverage float64, lastVisitedBefore time.Time) ([]model.RestedFavorite, error)
	PastFavorites(ctx context.Context, minAverage float64, lastVisitedBefore time.Time) ([]model.RestedFavorite, error)
	PickCounts(ctx context.Context, since time.Time) ([]model.PickCount, error)
}

func New(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
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

func nullableString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

var _ = time.Time{}
