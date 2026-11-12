package repository

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

// RestedFavorites mirrors the Postgres store: restaurants whose counted
// ratings average at least minAverage and whose latest visit was before
// lastVisitedBefore, by name.
func (m *MemoryStore) RestedFavorites(_ context.Context, minAverage float64, lastVisitedBefore time.Time) ([]model.RestedFavorite, error) {
	favorites := m.restedFavorites(minAverage, lastVisitedBefore)
	sortFavorites(favorites, func(a, b model.RestedFavorite) int { return 0 })
	return favorites, nil
}

// PastFavorites mirrors the Postgres store: RestedFavorites by highest average
// rating, then the longest since the last visit, then name.
func (m *MemoryStore) PastFavorites(_ context.Context, minAverage float64, lastVisitedBefore time.Time) ([]model.RestedFavorite, error) {
	favorites := m.restedFavorites(minAverage, lastVisitedBefore)
	sortFavorites(favorites, func(a, b model.RestedFavorite) int {
		return cmp.Or(
			cmp.Compare(b.AverageRating, a.AverageRating),
			a.LastVisitedAt.Compare(b.LastVisitedAt),
		)
	})
	return favorites, nil
}

// sortFavorites orders by first, then by name and ID.
func sortFavorites(favorites []model.RestedFavorite, first func(a, b model.RestedFavorite) int) {
	slices.SortFunc(favorites, func(a, b model.RestedFavorite) int {
		return cmp.Or(
			first(a, b),
			cmp.Compare(a.Restaurant.Name, b.Restaurant.Name),
			cmp.Compare(a.Restaurant.ID.String(), b.Restaurant.ID.String()),
		)
	})
}

func (m *MemoryStore) restedFavorites(minAverage float64, lastVisitedBefore time.Time) []model.RestedFavorite {
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
		if !ok || restaurant.IsClosed() {
			continue
		}
		favorites = append(favorites, model.RestedFavorite{Restaurant: restaurant, AverageRating: averageRating, LastVisitedAt: lastVisits[id]})
	}
	return favorites
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
