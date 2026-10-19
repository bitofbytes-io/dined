package repository

import (
	"context"
	"sort"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

// RestedFavorites mirrors the Postgres store: restaurants whose counted
// ratings average at least minAverage and whose latest visit was before
// lastVisitedBefore, by name.
func (m *MemoryStore) RestedFavorites(_ context.Context, minAverage float64, lastVisitedBefore time.Time) ([]model.RestedFavorite, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	scores := map[uuid.UUID][]float64{}
	lastVisits := map[uuid.UUID]time.Time{}
	for _, visit := range m.visits {
		id := visit.Restaurant.ID
		if visit.VisitedAt.After(lastVisits[id]) {
			lastVisits[id] = visit.VisitedAt
		}
		if !visit.CountsInAggregates() {
			continue
		}
		for _, rating := range visit.Ratings {
			scores[id] = append(scores[id], rating.Score)
		}
	}
	var favorites []model.RestedFavorite
	for id, restaurantScores := range scores {
		averageRating := average(restaurantScores)
		if averageRating < minAverage || !lastVisits[id].Before(lastVisitedBefore) {
			continue
		}
		restaurant, ok := m.findRestaurant(&id)
		if !ok {
			continue
		}
		favorites = append(favorites, model.RestedFavorite{Restaurant: restaurant, AverageRating: averageRating, LastVisitedAt: lastVisits[id]})
	}
	sort.Slice(favorites, func(i, j int) bool {
		if favorites[i].Restaurant.Name == favorites[j].Restaurant.Name {
			return favorites[i].Restaurant.ID.String() < favorites[j].Restaurant.ID.String()
		}
		return favorites[i].Restaurant.Name < favorites[j].Restaurant.Name
	})
	return favorites, nil
}

// PickCounts mirrors the Postgres store: each person's picks since a time and
// latest pick ever, in people order.
func (m *MemoryStore) PickCounts(_ context.Context, since time.Time) ([]model.PickCount, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	counts := make([]model.PickCount, len(m.people))
	byPerson := make(map[uuid.UUID]*model.PickCount, len(m.people))
	for i, person := range m.people {
		counts[i].Person = person
		byPerson[person.ID] = &counts[i]
	}
	for _, visit := range m.visits {
		if visit.Picker == nil {
			continue
		}
		count := byPerson[visit.Picker.ID]
		if count == nil {
			continue
		}
		if !visit.VisitedAt.Before(since) {
			count.Picks++
		}
		if count.LastPickedAt == nil || visit.VisitedAt.After(*count.LastPickedAt) {
			visitedAt := visit.VisitedAt
			count.LastPickedAt = &visitedAt
		}
	}
	return counts, nil
}
