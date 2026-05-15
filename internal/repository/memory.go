package repository

import (
	"context"
	"sort"
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
			GoogleRating:  floatPtr(4.1),
			Category:      strPtr("Indian"),
			IsChain:       true,
			CreatedAt:     now,
			UpdatedAt:     now,
			GooglePlaceID: strPtr("demo-saffron"),
		},
	}
	visits := []model.Visit{
		demoVisit(restaurants[0], people[0], now.AddDate(0, 0, -2), 2, "Still the safest fries order.", []model.Rating{{Person: people[0], Score: 8.5}, {Person: people[1], Score: 7}, {Person: people[2], Score: 8}, {Person: people[3], Score: 7.5}}, []model.Tag{tags[0], tags[1]}),
		demoVisit(restaurants[1], people[1], now.AddDate(0, 0, -9), 2, "Great salsa. Table was split on the enchiladas.", []model.Rating{{Person: people[0], Score: 9}, {Person: people[1], Score: 7}, {Person: people[2], Score: 8.5}, {Person: people[3], Score: 9}}, []model.Tag{tags[0], tags[2]}),
		demoVisit(restaurants[2], people[2], now.AddDate(0, 0, -15), 3, "Good bowls, but everyone wanted more naan.", []model.Rating{{Person: people[0], Score: 8}, {Person: people[1], Score: 8.5}, {Person: people[2], Score: 8}}, []model.Tag{tags[4]}),
	}
	return &MemoryStore{people: people, tags: tags, restaurants: restaurants, visits: visits}
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

func (m *MemoryStore) Restaurants(_ context.Context, q string) ([]model.Restaurant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	q = strings.ToLower(strings.TrimSpace(q))
	var restaurants []model.Restaurant
	for _, restaurant := range m.restaurants {
		if q == "" || strings.Contains(strings.ToLower(restaurant.Name), q) || (restaurant.Address != nil && strings.Contains(strings.ToLower(*restaurant.Address), q)) {
			restaurants = append(restaurants, restaurant)
		}
	}
	sort.Slice(restaurants, func(i, j int) bool { return restaurants[i].Name < restaurants[j].Name })
	return restaurants, nil
}

func (m *MemoryStore) Restaurant(_ context.Context, id uuid.UUID) (*model.Restaurant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, restaurant := range m.restaurants {
		if restaurant.ID == id {
			copy := restaurant
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *MemoryStore) Visits(_ context.Context, limit int) ([]model.Visit, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	visits := append([]model.Visit(nil), m.visits...)
	sortVisitsNewestFirst(visits)
	if limit > 0 && len(visits) > limit {
		visits = visits[:limit]
	}
	return visits, nil
}

func (m *MemoryStore) RestaurantVisits(_ context.Context, restaurantID uuid.UUID) ([]model.Visit, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var visits []model.Visit
	for _, visit := range m.visits {
		if visit.Restaurant.ID == restaurantID {
			visits = append(visits, visit)
		}
	}
	sortVisitsNewestFirst(visits)
	return visits, nil
}

func (m *MemoryStore) CreateVisit(_ context.Context, input model.VisitInput) (*uuid.UUID, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	restaurant, ok := m.findRestaurant(input.RestaurantID)
	if !ok {
		restaurant, ok = m.findRestaurantByInput(input)
		if !ok {
			now := time.Now()
			restaurant = model.Restaurant{
				ID:            uuid.New(),
				Name:          strings.TrimSpace(input.RestaurantName),
				Address:       strPtrOrNil(input.Address),
				GooglePlaceID: strPtrOrNil(input.GooglePlaceID),
				Category:      strPtrOrNil(input.Category),
				IsChain:       input.IsChain,
				CreatedAt:     now,
				UpdatedAt:     now,
			}
			m.restaurants = append(m.restaurants, restaurant)
		}
	}

	picker := m.personByID(input.PickerID)
	visit := model.Visit{
		ID:         uuid.New(),
		Restaurant: restaurant,
		VisitedAt:  input.VisitedAt,
		Picker:     picker,
		PriceLevel: input.PriceLevel,
		Notes:      strPtrOrNil(input.Notes),
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	for personID, score := range input.Ratings {
		if score == 0 {
			continue
		}
		visit.Ratings = append(visit.Ratings, model.Rating{Person: m.personByID(personID), Score: score})
	}
	for _, tagID := range input.TagIDs {
		if tag, ok := m.tagByID(tagID); ok {
			visit.Tags = append(visit.Tags, tag)
		}
	}
	if name := strings.TrimSpace(input.NewTag); name != "" {
		tag := model.Tag{ID: uuid.New(), Name: name}
		m.tags = append(m.tags, tag)
		visit.Tags = append(visit.Tags, tag)
	}
	m.visits = append(m.visits, visit)
	return &visit.ID, nil
}

func (m *MemoryStore) DeleteVisit(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, visit := range m.visits {
		if visit.ID == id {
			m.visits = append(m.visits[:i], m.visits[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *MemoryStore) ToggleChain(_ context.Context, id uuid.UUID, isChain bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.restaurants {
		if m.restaurants[i].ID == id {
			m.restaurants[i].IsChain = isChain
			m.restaurants[i].UpdatedAt = time.Now()
		}
	}
	for i := range m.visits {
		if m.visits[i].Restaurant.ID == id {
			m.visits[i].Restaurant.IsChain = isChain
		}
	}
	return nil
}

func (m *MemoryStore) Stats(context.Context) (model.Stats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var stats model.Stats
	stats.TotalDines = len(m.visits)
	var sum float64
	var count int
	visitsByRestaurant := map[string]int{}
	ratingByRestaurant := map[string][]float64{}
	ratingByPicker := map[string][]float64{}
	var biggestSplit float64
	for _, visit := range m.visits {
		visitsByRestaurant[visit.Restaurant.Name]++
		for _, rating := range visit.Ratings {
			sum += rating.Score
			count++
			ratingByRestaurant[visit.Restaurant.Name] = append(ratingByRestaurant[visit.Restaurant.Name], rating.Score)
			ratingByPicker[visit.Picker.Name] = append(ratingByPicker[visit.Picker.Name], rating.Score)
		}
		if split := visitSplit(visit); split > biggestSplit {
			biggestSplit = split
			stats.BiggestSplitRestaurant = visit.Restaurant.Name
		}
	}
	if count > 0 {
		stats.AverageRating = sum / float64(count)
	}
	stats.MostVisitedRestaurant = topCount(visitsByRestaurant)
	stats.HighestRatedRestaurant = topAverage(ratingByRestaurant)
	stats.BestPicker = topAverage(ratingByPicker)
	return stats, nil
}

func (m *MemoryStore) PickerTurn(context.Context) (model.PickerTurn, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.people) == 0 {
		return model.PickerTurn{}, nil
	}
	if len(m.visits) == 0 {
		return model.PickerTurn{NextPicker: m.people[0]}, nil
	}

	visits := append([]model.Visit(nil), m.visits...)
	sortVisitsNewestFirst(visits)
	last := visits[0].Picker
	next := m.people[0]
	for i, person := range m.people {
		if person.ID == last.ID {
			next = m.people[(i+1)%len(m.people)]
			break
		}
	}
	return model.PickerTurn{LastPicker: last, NextPicker: next}, nil
}

func sortVisitsNewestFirst(visits []model.Visit) {
	sort.Slice(visits, func(i, j int) bool {
		if visits[i].VisitedAt.Equal(visits[j].VisitedAt) {
			return visits[i].CreatedAt.After(visits[j].CreatedAt)
		}
		return visits[i].VisitedAt.After(visits[j].VisitedAt)
	})
}

func (m *MemoryStore) findRestaurant(id *uuid.UUID) (model.Restaurant, bool) {
	if id == nil {
		return model.Restaurant{}, false
	}
	for _, restaurant := range m.restaurants {
		if restaurant.ID == *id {
			return restaurant, true
		}
	}
	return model.Restaurant{}, false
}

func (m *MemoryStore) findRestaurantByInput(input model.VisitInput) (model.Restaurant, bool) {
	placeID := strings.TrimSpace(input.GooglePlaceID)
	name := strings.TrimSpace(input.RestaurantName)
	address := strings.TrimSpace(input.Address)
	for i := range m.restaurants {
		if placeID != "" && m.restaurants[i].GooglePlaceID != nil && *m.restaurants[i].GooglePlaceID == placeID {
			return m.restaurants[i], true
		}
	}
	for i := range m.restaurants {
		if address != "" && m.restaurants[i].Address != nil &&
			strings.EqualFold(strings.TrimSpace(m.restaurants[i].Name), name) &&
			strings.EqualFold(strings.TrimSpace(*m.restaurants[i].Address), address) {
			now := time.Now()
			if placeID != "" && m.restaurants[i].GooglePlaceID == nil {
				m.restaurants[i].GooglePlaceID = strPtr(placeID)
				m.restaurants[i].UpdatedAt = now
			}
			if category := strings.TrimSpace(input.Category); category != "" && m.restaurants[i].Category == nil {
				m.restaurants[i].Category = strPtr(category)
				m.restaurants[i].UpdatedAt = now
			}
			return m.restaurants[i], true
		}
	}
	return model.Restaurant{}, false
}

func (m *MemoryStore) personByID(id uuid.UUID) model.Person {
	for _, person := range m.people {
		if person.ID == id {
			return person
		}
	}
	return model.Person{}
}

func (m *MemoryStore) tagByID(id uuid.UUID) (model.Tag, bool) {
	for _, tag := range m.tags {
		if tag.ID == id {
			return tag, true
		}
	}
	return model.Tag{}, false
}

func demoVisit(restaurant model.Restaurant, picker model.Person, visitedAt time.Time, price int, notes string, ratings []model.Rating, tags []model.Tag) model.Visit {
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

func visitSplit(visit model.Visit) float64 {
	if len(visit.Ratings) < 2 {
		return 0
	}
	min := visit.Ratings[0].Score
	max := visit.Ratings[0].Score
	for _, rating := range visit.Ratings[1:] {
		if rating.Score < min {
			min = rating.Score
		}
		if rating.Score > max {
			max = rating.Score
		}
	}
	return max - min
}

func topCount(values map[string]int) string {
	bestName := ""
	bestValue := -1
	for name, value := range values {
		if value > bestValue || (value == bestValue && name < bestName) {
			bestName = name
			bestValue = value
		}
	}
	return bestName
}

func topAverage(values map[string][]float64) string {
	bestName := ""
	bestAvg := -1.0
	for name, scores := range values {
		var sum float64
		for _, score := range scores {
			sum += score
		}
		avg := sum / float64(len(scores))
		if avg > bestAvg || (avg == bestAvg && name < bestName) {
			bestName = name
			bestAvg = avg
		}
	}
	return bestName
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
