package repository

import (
	"context"
	"errors"
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
			City:          strPtr("Raleigh"),
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

func (m *MemoryStore) Visit(_ context.Context, id uuid.UUID) (*model.Visit, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, visit := range m.visits {
		if visit.ID == id {
			copy := visit
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
		}
	} else if input.RestaurantID != nil {
		m.updateRestaurantMetadataByID(*input.RestaurantID, input)
		restaurant, _ = m.findRestaurant(input.RestaurantID)
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
	visit.Ratings = m.ratingsFromInput(input)
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

func (m *MemoryStore) UpdateRestaurantGoogleMetadata(_ context.Context, id uuid.UUID, metadata model.GoogleRestaurantMetadata) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.restaurants {
		if m.restaurants[i].ID == id {
			m.refreshGoogleMetadata(i, metadata)
			return nil
		}
	}
	return nil
}

func (m *MemoryStore) UpdateVisit(_ context.Context, id uuid.UUID, input model.VisitInput) error {
	if err := input.Validate(); err != nil {
		return err
	}
	if input.RestaurantID == nil {
		return errors.New("restaurant is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	restaurant, ok := m.findRestaurant(input.RestaurantID)
	if !ok {
		return errors.New("restaurant not found")
	}
	picker := m.personByID(input.PickerID)
	if picker.ID == uuid.Nil {
		return errors.New("picker not found")
	}
	visitIndex := -1
	for i := range m.visits {
		if m.visits[i].ID == id {
			visitIndex = i
			break
		}
	}
	if visitIndex == -1 {
		return errors.New("visit not found")
	}

	ratings := m.ratingsFromInput(input)

	tagIDs := append([]uuid.UUID(nil), input.TagIDs...)
	if name := strings.TrimSpace(input.NewTag); name != "" {
		tag := model.Tag{ID: uuid.New(), Name: name}
		m.tags = append(m.tags, tag)
		tagIDs = append(tagIDs, tag.ID)
	}
	var tags []model.Tag
	for _, tagID := range tagIDs {
		if tag, ok := m.tagByID(tagID); ok {
			tags = append(tags, tag)
		}
	}

	m.visits[visitIndex].Restaurant = restaurant
	m.visits[visitIndex].VisitedAt = input.VisitedAt
	m.visits[visitIndex].Picker = picker
	m.visits[visitIndex].PriceLevel = input.PriceLevel
	m.visits[visitIndex].Notes = strPtrOrNil(input.Notes)
	m.visits[visitIndex].Ratings = ratings
	m.visits[visitIndex].Tags = tags
	m.visits[visitIndex].UpdatedAt = time.Now()
	return nil
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

func (m *MemoryStore) Stats(context.Context) (model.Stats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var stats model.Stats
	stats.TotalDines = len(m.visits)
	var sum float64
	var count int
	visitedRestaurants := map[uuid.UUID]struct{}{}
	cities := map[string]struct{}{}
	restaurantCities := map[uuid.UUID]string{}
	for _, restaurant := range m.restaurants {
		restaurantCities[restaurant.ID] = strings.TrimSpace(valueOrEmpty(restaurant.City))
	}
	visitsByRestaurant := map[string]int{}
	ratingByRestaurant := map[string][]float64{}
	ratingsByRestaurantID := map[uuid.UUID]*restaurantRatingAggregate{}
	ratingsByCuisineRestaurantID := map[string]map[uuid.UUID]*restaurantRatingAggregate{}
	ratingByPicker := map[string][]float64{}
	var biggestSplit float64
	for _, visit := range m.visits {
		visitedRestaurants[visit.Restaurant.ID] = struct{}{}
		city := restaurantCities[visit.Restaurant.ID]
		if city == "" {
			city = strings.TrimSpace(valueOrEmpty(visit.Restaurant.City))
		}
		if city != "" {
			cities[strings.ToLower(city)] = struct{}{}
		}
		visitsByRestaurant[visit.Restaurant.Name]++
		restaurantAggregate := ratingsByRestaurantID[visit.Restaurant.ID]
		if restaurantAggregate == nil {
			restaurantAggregate = &restaurantRatingAggregate{name: visit.Restaurant.Name}
			ratingsByRestaurantID[visit.Restaurant.ID] = restaurantAggregate
		}
		restaurantAggregate.visitCount++
		cuisine := strings.TrimSpace(valueOrEmpty(visit.Restaurant.Category))
		var cuisineAggregate *restaurantRatingAggregate
		if cuisine != "" {
			cuisineKey := strings.ToLower(cuisine)
			cuisineRestaurants := ratingsByCuisineRestaurantID[cuisineKey]
			if cuisineRestaurants == nil {
				cuisineRestaurants = map[uuid.UUID]*restaurantRatingAggregate{}
				ratingsByCuisineRestaurantID[cuisineKey] = cuisineRestaurants
			}
			cuisineAggregate = cuisineRestaurants[visit.Restaurant.ID]
			if cuisineAggregate == nil {
				cuisineAggregate = &restaurantRatingAggregate{name: visit.Restaurant.Name, cuisine: cuisine}
				cuisineRestaurants[visit.Restaurant.ID] = cuisineAggregate
			}
			cuisineAggregate.visitCount++
		}
		for _, rating := range visit.Ratings {
			sum += rating.Score
			count++
			ratingByRestaurant[visit.Restaurant.Name] = append(ratingByRestaurant[visit.Restaurant.Name], rating.Score)
			restaurantAggregate.ratings = append(restaurantAggregate.ratings, rating.Score)
			if cuisineAggregate != nil {
				cuisineAggregate.ratings = append(cuisineAggregate.ratings, rating.Score)
			}
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
	stats.BestPicker, stats.BestPickerAverage = topAverageWithScore(ratingByPicker)
	stats.NewPlaces = len(visitedRestaurants)
	stats.CitiesExplored = len(cities)
	stats.TopRestaurants = topRestaurantStats(ratingsByRestaurantID)
	stats.TopRestaurantsByCuisine = topRestaurantStatsByCuisine(ratingsByCuisineRestaurantID)
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
	now := time.Now()
	if placeID := strings.TrimSpace(input.GooglePlaceID); placeID != "" && m.restaurants[index].GooglePlaceID == nil {
		m.restaurants[index].GooglePlaceID = strPtr(placeID)
		m.restaurants[index].UpdatedAt = now
	}
	if category := strings.TrimSpace(input.Category); category != "" && m.restaurants[index].Category == nil {
		m.restaurants[index].Category = strPtr(category)
		m.restaurants[index].UpdatedAt = now
	}
	if city := strings.TrimSpace(input.City); city != "" && m.restaurants[index].City == nil {
		m.restaurants[index].City = strPtr(city)
		m.restaurants[index].UpdatedAt = now
	}
	m.updateGoogleMetadata(index, input.GoogleMetadata)
}

func (m *MemoryStore) updateRestaurantMetadataByID(id uuid.UUID, input model.VisitInput) {
	for i := range m.restaurants {
		if m.restaurants[i].ID == id {
			m.updateRestaurantMetadata(i, input)
			return
		}
	}
}

func (m *MemoryStore) updateGoogleMetadata(index int, metadata model.GoogleRestaurantMetadata) {
	now := time.Now()
	if metadata.Latitude != nil && m.restaurants[index].Latitude == nil {
		m.restaurants[index].Latitude = floatPtr(*metadata.Latitude)
		m.restaurants[index].UpdatedAt = now
	}
	if metadata.Longitude != nil && m.restaurants[index].Longitude == nil {
		m.restaurants[index].Longitude = floatPtr(*metadata.Longitude)
		m.restaurants[index].UpdatedAt = now
	}
	if phone := strings.TrimSpace(metadata.Phone); phone != "" && m.restaurants[index].Phone == nil {
		m.restaurants[index].Phone = strPtr(phone)
		m.restaurants[index].UpdatedAt = now
	}
	if website := strings.TrimSpace(metadata.Website); website != "" && m.restaurants[index].Website == nil {
		m.restaurants[index].Website = strPtr(website)
		m.restaurants[index].UpdatedAt = now
	}
	if metadata.GoogleRating != nil && m.restaurants[index].GoogleRating == nil {
		m.restaurants[index].GoogleRating = floatPtr(*metadata.GoogleRating)
		m.restaurants[index].UpdatedAt = now
	}
	if metadata.GooglePriceLevel != nil && m.restaurants[index].GooglePriceLevel == nil {
		m.restaurants[index].GooglePriceLevel = intPtr(*metadata.GooglePriceLevel)
		m.restaurants[index].UpdatedAt = now
	}
	m.syncRestaurant(m.restaurants[index])
}

func (m *MemoryStore) refreshGoogleMetadata(index int, metadata model.GoogleRestaurantMetadata) {
	now := time.Now()
	if metadata.Latitude != nil {
		m.restaurants[index].Latitude = floatPtr(*metadata.Latitude)
		m.restaurants[index].UpdatedAt = now
	}
	if metadata.Longitude != nil {
		m.restaurants[index].Longitude = floatPtr(*metadata.Longitude)
		m.restaurants[index].UpdatedAt = now
	}
	if phone := strings.TrimSpace(metadata.Phone); phone != "" {
		m.restaurants[index].Phone = strPtr(phone)
		m.restaurants[index].UpdatedAt = now
	}
	if website := strings.TrimSpace(metadata.Website); website != "" {
		m.restaurants[index].Website = strPtr(website)
		m.restaurants[index].UpdatedAt = now
	}
	if metadata.GoogleRating != nil {
		m.restaurants[index].GoogleRating = floatPtr(*metadata.GoogleRating)
		m.restaurants[index].UpdatedAt = now
	}
	if metadata.GooglePriceLevel != nil {
		m.restaurants[index].GooglePriceLevel = intPtr(*metadata.GooglePriceLevel)
		m.restaurants[index].UpdatedAt = now
	}
	m.syncRestaurant(m.restaurants[index])
}

func (m *MemoryStore) personByID(id uuid.UUID) model.Person {
	for _, person := range m.people {
		if person.ID == id {
			return person
		}
	}
	return model.Person{}
}

func (m *MemoryStore) ratingsFromInput(input model.VisitInput) []model.Rating {
	var ratings []model.Rating
	seen := map[uuid.UUID]struct{}{}
	for _, person := range m.people {
		score, ok := input.Ratings[person.ID]
		if !ok {
			continue
		}
		ratings = append(ratings, model.Rating{Person: person, Score: score})
		seen[person.ID] = struct{}{}
	}
	for personID, score := range input.Ratings {
		if _, ok := seen[personID]; ok {
			continue
		}
		ratings = append(ratings, model.Rating{Person: m.personByID(personID), Score: score})
	}
	return ratings
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
	name, _ := topAverageWithScore(values)
	return name
}

func topAverageWithScore(values map[string][]float64) (string, float64) {
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
	if bestAvg < 0 {
		return "", 0
	}
	return bestName, bestAvg
}

type restaurantRatingAggregate struct {
	cuisine    string
	name       string
	visitCount int
	ratings    []float64
}

func topRestaurantStats(values map[uuid.UUID]*restaurantRatingAggregate) []model.RestaurantRatingStat {
	var restaurants []model.RestaurantRatingStat
	for _, value := range values {
		if len(value.ratings) < 2 {
			continue
		}
		restaurants = append(restaurants, model.RestaurantRatingStat{
			Name:          value.name,
			AverageRating: average(value.ratings),
			RatingCount:   len(value.ratings),
			VisitCount:    value.visitCount,
		})
	}
	sort.Slice(restaurants, func(i, j int) bool {
		if restaurants[i].AverageRating != restaurants[j].AverageRating {
			return restaurants[i].AverageRating > restaurants[j].AverageRating
		}
		if restaurants[i].RatingCount != restaurants[j].RatingCount {
			return restaurants[i].RatingCount > restaurants[j].RatingCount
		}
		return restaurants[i].Name < restaurants[j].Name
	})
	if len(restaurants) > 5 {
		restaurants = restaurants[:5]
	}
	return restaurants
}

func topRestaurantStatsByCuisine(values map[string]map[uuid.UUID]*restaurantRatingAggregate) []model.CuisineRestaurantStat {
	var restaurants []model.CuisineRestaurantStat
	for _, cuisineRestaurants := range values {
		candidates := topRestaurantStats(cuisineRestaurants)
		if len(candidates) == 0 {
			continue
		}
		winner := candidates[0]
		cuisine := ""
		for _, value := range cuisineRestaurants {
			if value.name == winner.Name {
				cuisine = value.cuisine
				break
			}
		}
		restaurants = append(restaurants, model.CuisineRestaurantStat{
			Cuisine:       cuisine,
			Name:          winner.Name,
			AverageRating: winner.AverageRating,
			RatingCount:   winner.RatingCount,
			VisitCount:    winner.VisitCount,
		})
	}
	sort.Slice(restaurants, func(i, j int) bool {
		return restaurants[i].Cuisine < restaurants[j].Cuisine
	})
	return restaurants
}

func average(scores []float64) float64 {
	if len(scores) == 0 {
		return 0
	}
	var sum float64
	for _, score := range scores {
		sum += score
	}
	return sum / float64(len(scores))
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

func (m *MemoryStore) syncRestaurant(restaurant model.Restaurant) {
	for i := range m.visits {
		if m.visits[i].Restaurant.ID == restaurant.ID {
			m.visits[i].Restaurant = restaurant
		}
	}
}
