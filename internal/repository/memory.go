package repository

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

type MemoryStore struct {
	mu          sync.RWMutex
	people      []model.Person
	tags        []model.Tag
	restaurants []model.Restaurant
	visits      []model.Visit
	wantToTry   []memoryWantToTry
}

func NewMemoryStore() *MemoryStore {
	now := time.Now()
	people := []model.Person{
		{ID: uuid.New(), Name: "Daniel", AvatarColor: "#0d6f6f"},
		{ID: uuid.New(), Name: "Jen", AvatarColor: "#c7332f"},
		{ID: uuid.New(), Name: "Caleb", AvatarColor: "#e5a72f"},
		{ID: uuid.New(), Name: "Aiden", AvatarColor: "#2f8f6d"},
	}
	tags := []model.Tag{
		{ID: uuid.New(), Name: "Would Return"},
		{ID: uuid.New(), Name: "Long Wait"},
		{ID: uuid.New(), Name: "Great Service"},
		{ID: uuid.New(), Name: "Overpriced"},
		{ID: uuid.New(), Name: "Kid Approved"},
	}
	restaurants := []model.Restaurant{
		{
			ID:            uuid.New(),
			Name:          "Hank's Downtown Diner",
			Address:       strPtr("101 Main Street"),
			City:          strPtr("Raleigh"),
			Latitude:      floatPtr(35.7796),
			Longitude:     floatPtr(-78.6382),
			GoogleRating:  floatPtr(4.3),
			Category:      strPtr("American"),
			IsChain:       false,
			CreatedAt:     now,
			UpdatedAt:     now,
			GooglePlaceID: strPtr("demo-hanks"),
		},
		{
			ID:            uuid.New(),
			Name:          "El Patio Verde",
			Address:       strPtr("42 Garden Avenue"),
			City:          strPtr("Apex"),
			Latitude:      floatPtr(35.7327),
			Longitude:     floatPtr(-78.8503),
			GoogleRating:  floatPtr(4.6),
			Category:      strPtr("Mexican"),
			IsChain:       false,
			CreatedAt:     now,
			UpdatedAt:     now,
			GooglePlaceID: strPtr("demo-patio"),
		},
		{
			ID:            uuid.New(),
			Name:          "Saffron Counter",
			Address:       strPtr("8 Market Lane"),
			City:          strPtr("Cary"),
			Latitude:      floatPtr(35.7915),
			Longitude:     floatPtr(-78.7811),
			GoogleRating:  floatPtr(4.1),
			Category:      strPtr("Indian"),
			IsChain:       true,
			CreatedAt:     now,
			UpdatedAt:     now,
			GooglePlaceID: strPtr("demo-saffron"),
		},
	}
	visits := []model.Visit{
		demoVisit(restaurants[0], &people[0], now.AddDate(0, 0, -2), 2, "Still the safest fries order.", []model.Rating{{Person: people[0], Score: 8.5}, {Person: people[1], Score: 7}, {Person: people[2], Score: 8}, {Person: people[3], Score: 7.5}}, []model.Tag{tags[0], tags[1]}),
		demoVisit(restaurants[1], &people[1], now.AddDate(0, 0, -9), 2, "Great salsa. Table was split on the enchiladas.", []model.Rating{{Person: people[0], Score: 9}, {Person: people[1], Score: 7}, {Person: people[2], Score: 8.5}, {Person: people[3], Score: 9}}, []model.Tag{tags[0], tags[2]}),
		demoVisit(restaurants[2], &people[2], now.AddDate(0, 0, -15), 3, "Good bowls, but everyone wanted more naan.", []model.Rating{{Person: people[0], Score: 8}, {Person: people[1], Score: 8.5}, {Person: people[2], Score: 8}}, []model.Tag{tags[4]}),
	}
	return &MemoryStore{people: people, tags: tags, restaurants: restaurants, visits: visits}
}

// Ping always succeeds because the memory store has no database to reach.
func (m *MemoryStore) Ping(context.Context) error {
	return nil
}

func (m *MemoryStore) People(context.Context) ([]model.Person, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]model.Person(nil), m.people...), nil
}

func (m *MemoryStore) Tags(context.Context) ([]model.Tag, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]model.Tag(nil), m.tags...), nil
}

// demoVisit builds a seeded visit; a nil picker means Everybody picked.
func demoVisit(restaurant model.Restaurant, picker *model.Person, visitedAt time.Time, price int, notes string, ratings []model.Rating, tags []model.Tag) model.Visit {
	now := time.Now()
	return model.Visit{
		ID:         uuid.New(),
		Restaurant: restaurant,
		VisitedAt:  visitedAt,
		Picker:     picker,
		PriceLevel: price,
		Notes:      &notes,
		Ratings:    ratings,
		Tags:       tags,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func strPtr(value string) *string {
	return &value
}

func strPtrOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func floatPtr(value float64) *float64 {
	return &value
}

func floatPtrOrNil(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func intPtr(value int) *int {
	return &value
}

func intPtrOrNil(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
