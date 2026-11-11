package repository

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

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

func (m *MemoryStore) RestaurantsByPlaceIDs(_ context.Context, placeIDs []string) (map[string]model.Restaurant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	restaurants := map[string]model.Restaurant{}
	for _, restaurant := range m.restaurants {
		if restaurant.GooglePlaceID != nil && slices.Contains(placeIDs, *restaurant.GooglePlaceID) {
			restaurants[*restaurant.GooglePlaceID] = restaurant
		}
	}
	return restaurants, nil
}

func (m *MemoryStore) VisitedRestaurantMapPoints(context.Context) ([]model.RestaurantMapPoint, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	byRestaurant := map[uuid.UUID]model.RestaurantMapPoint{}
	for _, visit := range m.visits {
		if visit.Restaurant.Latitude == nil || visit.Restaurant.Longitude == nil {
			continue
		}
		point := byRestaurant[visit.Restaurant.ID]
		if point.RestaurantID == uuid.Nil {
			point = model.RestaurantMapPoint{
				RestaurantID: visit.Restaurant.ID,
				Name:         visit.Restaurant.Name,
				Latitude:     *visit.Restaurant.Latitude,
				Longitude:    *visit.Restaurant.Longitude,
			}
		}
		point.VisitCount++
		if visit.VisitedAt.After(point.LatestVisitedAt) {
			point.LatestVisitedAt = visit.VisitedAt
		}
		byRestaurant[visit.Restaurant.ID] = point
	}

	points := make([]model.RestaurantMapPoint, 0, len(byRestaurant))
	for _, point := range byRestaurant {
		points = append(points, point)
	}
	sort.Slice(points, func(i, j int) bool {
		if !points[i].LatestVisitedAt.Equal(points[j].LatestVisitedAt) {
			return points[i].LatestVisitedAt.After(points[j].LatestVisitedAt)
		}
		return points[i].Name < points[j].Name
	})
	return points, nil
}

// resolveRestaurant mirrors the Postgres resolveRestaurant: the chosen
// restaurant, else one matched by Google place ID, else one matched by name
// and address, else a newly added one. Callers hold m.mu for writing.
func (m *MemoryStore) resolveRestaurant(input model.VisitInput) (model.Restaurant, error) {
	if restaurant, ok := m.findRestaurant(input.RestaurantID); ok {
		if err := m.checkChosenRestaurantPlace(restaurant, input.GooglePlaceID); err != nil {
			return model.Restaurant{}, err
		}
		m.updateRestaurantMetadataByID(restaurant.ID, withoutPlaceDetails(input))
		restaurant, _ = m.findRestaurant(input.RestaurantID)
		return restaurant, nil
	}
	if restaurant, ok := m.findRestaurantByInput(input); ok {
		return restaurant, nil
	}
	now := time.Now()
	restaurant := model.Restaurant{
		ID:               uuid.New(),
		Name:             strings.TrimSpace(input.RestaurantName),
		Address:          strPtrOrNil(input.Address),
		City:             strPtrOrNil(input.City),
		Latitude:         floatPtrOrNil(input.GoogleMetadata.Latitude),
		Longitude:        floatPtrOrNil(input.GoogleMetadata.Longitude),
		Phone:            strPtrOrNil(input.GoogleMetadata.Phone),
		Website:          strPtrOrNil(input.GoogleMetadata.Website),
		GooglePlaceID:    strPtrOrNil(input.GooglePlaceID),
		GoogleRating:     floatPtrOrNil(input.GoogleMetadata.GoogleRating),
		GooglePriceLevel: intPtrOrNil(input.GoogleMetadata.GooglePriceLevel),
		Category:         strPtrOrNil(input.Category),
		IsChain:          input.IsChain,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	m.restaurants = append(m.restaurants, restaurant)
	return restaurant, nil
}

func (m *MemoryStore) UpdateRestaurantGoogleMetadata(_ context.Context, id uuid.UUID, metadata model.GoogleRestaurantMetadata) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.restaurants {
		if m.restaurants[i].ID == id {
			m.applyGoogleMetadata(i, metadata, true)
			return nil
		}
	}
	return nil
}

func (m *MemoryStore) DeleteRestaurantIfUnvisited(_ context.Context, id uuid.UUID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, visit := range m.visits {
		if visit.Restaurant.ID == id {
			return false, nil
		}
	}
	for i, restaurant := range m.restaurants {
		if restaurant.ID == id {
			m.restaurants = append(m.restaurants[:i], m.restaurants[i+1:]...)
			m.deleteRestaurantWantToTry(id)
			return true, nil
		}
	}
	return false, nil
}

func (m *MemoryStore) UpdateRestaurant(_ context.Context, id uuid.UUID, input model.RestaurantInput) error {
	if err := input.Validate(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.restaurants {
		if m.restaurants[i].ID != id {
			continue
		}
		m.restaurants[i].Name = strings.TrimSpace(input.Name)
		m.restaurants[i].Address = strPtrOrNil(input.Address)
		m.restaurants[i].City = strPtrOrNil(input.City)
		m.restaurants[i].Phone = strPtrOrNil(input.Phone)
		m.restaurants[i].Website = strPtrOrNil(input.Website)
		m.restaurants[i].GoogleRating = floatPtrOrNil(input.GoogleRating)
		m.restaurants[i].GooglePriceLevel = intPtrOrNil(input.GooglePriceLevel)
		m.restaurants[i].Category = strPtrOrNil(input.Category)
		m.restaurants[i].IsChain = input.IsChain
		switch {
		case !input.Closed:
			m.restaurants[i].ClosedAt = nil
		case m.restaurants[i].ClosedAt == nil:
			closedAt := closingDate(time.Now())
			m.restaurants[i].ClosedAt = &closedAt
		}
		m.restaurants[i].UpdatedAt = time.Now()
		m.syncRestaurant(m.restaurants[i])
		return nil
	}
	return errors.New("restaurant not found")
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

// checkChosenRestaurantPlace mirrors the Postgres check: a chosen restaurant
// only accepts its own Google place ID, or an unowned one when it has none.
func (m *MemoryStore) checkChosenRestaurantPlace(chosen model.Restaurant, googlePlaceID string) error {
	placeID := strings.TrimSpace(googlePlaceID)
	if placeID == "" || (chosen.GooglePlaceID != nil && *chosen.GooglePlaceID == placeID) {
		return nil
	}
	for _, restaurant := range m.restaurants {
		if restaurant.ID != chosen.ID && restaurant.GooglePlaceID != nil && *restaurant.GooglePlaceID == placeID {
			return &model.PlaceConflictError{Restaurant: chosen.Name, Owner: restaurant.Name}
		}
	}
	if chosen.GooglePlaceID != nil {
		return &model.PlaceConflictError{Restaurant: chosen.Name}
	}
	return nil
}

func (m *MemoryStore) findRestaurantByInput(input model.VisitInput) (model.Restaurant, bool) {
	placeID := strings.TrimSpace(input.GooglePlaceID)
	name := strings.TrimSpace(input.RestaurantName)
	address := strings.TrimSpace(input.Address)
	for i := range m.restaurants {
		if placeID != "" && m.restaurants[i].GooglePlaceID != nil && *m.restaurants[i].GooglePlaceID == placeID {
			m.updateRestaurantMetadata(i, input)
			return m.restaurants[i], true
		}
	}
	for i := range m.restaurants {
		if address != "" && m.restaurants[i].Address != nil &&
			strings.EqualFold(strings.TrimSpace(m.restaurants[i].Name), name) &&
			strings.EqualFold(strings.TrimSpace(*m.restaurants[i].Address), address) {
			m.updateRestaurantMetadata(i, input)
			return m.restaurants[i], true
		}
	}
	return model.Restaurant{}, false
}

func (m *MemoryStore) updateRestaurantMetadata(index int, input model.VisitInput) {
	if placeID := strings.TrimSpace(input.GooglePlaceID); placeID != "" && m.restaurants[index].GooglePlaceID == nil {
		m.restaurants[index].GooglePlaceID = strPtr(placeID)
		m.restaurants[index].UpdatedAt = time.Now()
	}
	if city := strings.TrimSpace(input.City); city != "" && m.restaurants[index].City == nil {
		m.restaurants[index].City = strPtr(city)
		m.restaurants[index].UpdatedAt = time.Now()
	}
	metadata := input.GoogleMetadata
	metadata.Category = input.Category
	m.applyGoogleMetadata(index, metadata, false)
}

func (m *MemoryStore) updateRestaurantMetadataByID(id uuid.UUID, input model.VisitInput) {
	for i := range m.restaurants {
		if m.restaurants[i].ID == id {
			m.updateRestaurantMetadata(i, input)
			return
		}
	}
}

// applyGoogleMetadata copies the metadata's non-empty fields onto a
// restaurant. With overwrite false it only fills fields that are still empty,
// like Postgres CreateVisit; with overwrite true it replaces them, like
// Postgres UpdateRestaurantGoogleMetadata.
func (m *MemoryStore) applyGoogleMetadata(index int, metadata model.GoogleRestaurantMetadata, overwrite bool) {
	restaurant := &m.restaurants[index]
	now := time.Now()
	setFloat := func(field **float64, value *float64) {
		if value != nil && (overwrite || *field == nil) {
			*field = floatPtr(*value)
			restaurant.UpdatedAt = now
		}
	}
	setString := func(field **string, value string) {
		if value = strings.TrimSpace(value); value != "" && (overwrite || *field == nil) {
			*field = strPtr(value)
			restaurant.UpdatedAt = now
		}
	}
	setFloat(&restaurant.Latitude, metadata.Latitude)
	setFloat(&restaurant.Longitude, metadata.Longitude)
	setString(&restaurant.Phone, metadata.Phone)
	setString(&restaurant.Website, metadata.Website)
	setFloat(&restaurant.GoogleRating, metadata.GoogleRating)
	if metadata.GooglePriceLevel != nil && (overwrite || restaurant.GooglePriceLevel == nil) {
		restaurant.GooglePriceLevel = intPtr(*metadata.GooglePriceLevel)
		restaurant.UpdatedAt = now
	}
	setString(&restaurant.Category, metadata.Category)
	m.syncRestaurant(*restaurant)
}

func (m *MemoryStore) syncRestaurant(restaurant model.Restaurant) {
	for i := range m.visits {
		if m.visits[i].Restaurant.ID == restaurant.ID {
			m.visits[i].Restaurant = restaurant
		}
	}
}
