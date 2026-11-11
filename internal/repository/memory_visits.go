package repository

import (
	"bytes"
	"context"
	"slices"
	"sort"
	"strings"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

func (m *MemoryStore) Visit(_ context.Context, id uuid.UUID) (*model.Visit, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, visit := range m.visits {
		if visit.ID == id {
			copy := visit
			copy.Photos = photoMetadata(visit.Photos)
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *MemoryStore) Visits(ctx context.Context, limit int) ([]model.Visit, error) {
	return m.VisitsPage(ctx, model.VisitFilter{}, limit, 0)
}

// VisitsPage mirrors the Postgres store: matching visits newest first, photo metadata without image data.
func (m *MemoryStore) VisitsPage(_ context.Context, filter model.VisitFilter, limit, offset int) ([]model.Visit, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	visits := m.matchingVisits(filter)
	sortVisitsNewestFirst(visits)
	visits = visits[min(max(offset, 0), len(visits)):]
	if limit > 0 && len(visits) > limit {
		visits = visits[:limit]
	}
	for i := range visits {
		visits[i].Photos = photoMetadata(visits[i].Photos)
	}
	return visits, nil
}

func (m *MemoryStore) CountVisits(_ context.Context, filter model.VisitFilter) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.matchingVisits(filter)), nil
}

// VisitFilterOptions mirrors the Postgres store: trimmed values from visited restaurants,
// one spelling per case-insensitive value, and visit years newest first.
func (m *MemoryStore) VisitFilterOptions(context.Context) (model.VisitFilterOptions, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cuisines, cities := map[string]string{}, map[string]string{}
	years := map[int]bool{}
	keep := func(values map[string]string, value *string) {
		trimmed := strings.TrimSpace(valueOrEmpty(value))
		if trimmed == "" {
			return
		}
		key := strings.ToLower(trimmed)
		if existing, ok := values[key]; !ok || trimmed < existing {
			values[key] = trimmed
		}
	}
	for _, visit := range m.visits {
		keep(cuisines, visit.Restaurant.Category)
		keep(cities, visit.Restaurant.City)
		years[visitYear(visit)] = true
	}
	var options model.VisitFilterOptions
	options.Cuisines = sortedValues(cuisines)
	options.Cities = sortedValues(cities)
	for year := range years {
		options.Years = append(options.Years, year)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(options.Years)))
	return options, nil
}

// sortedValues returns the spellings in values ordered by their lowercase keys.
func sortedValues(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	sorted := make([]string, len(keys))
	for i, key := range keys {
		sorted[i] = values[key]
	}
	return sorted
}

// matchingVisits copies the visits that match filter, in no particular order. Callers hold the lock.
func (m *MemoryStore) matchingVisits(filter model.VisitFilter) []model.Visit {
	var visits []model.Visit
	for _, visit := range m.visits {
		if visitMatches(visit, filter) {
			visits = append(visits, visit)
		}
	}
	return visits
}

// visitMatches applies a VisitFilter the way the Postgres store's visitFilterSQL does.
func visitMatches(visit model.Visit, filter model.VisitFilter) bool {
	if cuisine := strings.TrimSpace(filter.Cuisine); cuisine != "" && !strings.EqualFold(strings.TrimSpace(valueOrEmpty(visit.Restaurant.Category)), cuisine) {
		return false
	}
	if filter.EverybodyPicked {
		if visit.Picker != nil {
			return false
		}
	} else if filter.PickerID != nil && (visit.Picker == nil || visit.Picker.ID != *filter.PickerID) {
		return false
	}
	if filter.RaterID != nil && !slices.ContainsFunc(visit.Ratings, func(rating model.Rating) bool { return rating.Person.ID == *filter.RaterID }) {
		return false
	}
	if filter.TagID != nil && !slices.ContainsFunc(visit.Tags, func(tag model.Tag) bool { return tag.ID == *filter.TagID }) {
		return false
	}
	if city := strings.TrimSpace(filter.City); city != "" && !strings.EqualFold(strings.TrimSpace(valueOrEmpty(visit.Restaurant.City)), city) {
		return false
	}
	return filter.Year == 0 || visitYear(visit) == filter.Year
}

// visitYear is the calendar year of the visit in Eastern time.
func visitYear(visit model.Visit) int {
	return visit.VisitedAt.In(apptime.EasternLocation()).Year()
}

func (m *MemoryStore) VisitPosition(_ context.Context, id uuid.UUID) (int, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	visits := append([]model.Visit(nil), m.visits...)
	sortVisitsNewestFirst(visits)
	for i, visit := range visits {
		if visit.ID == id {
			return i, true, nil
		}
	}
	return 0, false, nil
}

func (m *MemoryStore) RestaurantVisits(_ context.Context, restaurantID uuid.UUID) ([]model.Visit, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var visits []model.Visit
	for _, visit := range m.visits {
		if visit.Restaurant.ID == restaurantID {
			visit.Photos = photoMetadata(visit.Photos)
			visits = append(visits, visit)
		}
	}
	sortVisitsNewestFirst(visits)
	return visits, nil
}

func (m *MemoryStore) RestaurantVisitSummaries(_ context.Context, restaurantIDs []uuid.UUID) (map[uuid.UUID][]model.Visit, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	wanted := make(map[uuid.UUID]bool, len(restaurantIDs))
	for _, id := range restaurantIDs {
		wanted[id] = true
	}
	summaries := map[uuid.UUID][]model.Visit{}
	for _, visit := range m.visits {
		if wanted[visit.Restaurant.ID] {
			summary := visit
			summary.Photos = nil
			summaries[visit.Restaurant.ID] = append(summaries[visit.Restaurant.ID], summary)
		}
	}
	for _, visits := range summaries {
		sortVisitsNewestFirst(visits)
	}
	return summaries, nil
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

func sortVisitsNewestFirst(visits []model.Visit) {
	sort.Slice(visits, func(i, j int) bool {
		if visits[i].VisitedAt.Equal(visits[j].VisitedAt) {
			if visits[i].CreatedAt.Equal(visits[j].CreatedAt) {
				return bytes.Compare(visits[i].ID[:], visits[j].ID[:]) < 0
			}
			return visits[i].CreatedAt.After(visits[j].CreatedAt)
		}
		return visits[i].VisitedAt.After(visits[j].VisitedAt)
	})
}
