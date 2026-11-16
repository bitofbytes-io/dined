package repository

import (
	"context"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

// YearInReview mirrors the Postgres store: the Eastern calendar year's visits
// by month, new and repeat restaurants, the most visited and highest rated
// restaurants, and each person's best-rated pick.
func (m *MemoryStore) YearInReview(_ context.Context, year int) (model.YearInReview, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	review := model.YearInReview{Year: year}
	start, end := apptime.YearBounds(year)

	type restaurantYear struct {
		name   string
		visits int
		lastAt int64
		// Ratings come only from visits whose ratings count in aggregates.
		ratings      []float64
		countedVisit int
	}
	byRestaurant := map[uuid.UUID]*restaurantYear{}
	// firstVisit is each restaurant's earliest visit in any year.
	firstVisit := map[uuid.UUID]int64{}
	for _, visit := range m.visits {
		at := visit.VisitedAt.UnixNano()
		if first, ok := firstVisit[visit.Restaurant.ID]; !ok || at < first {
			firstVisit[visit.Restaurant.ID] = at
		}
	}
	type pickKey struct{ person, restaurant uuid.UUID }
	type pickYear struct {
		restaurant string
		ratings    []float64
	}
	picks := map[pickKey]*pickYear{}
	for _, visit := range m.visits {
		if visit.VisitedAt.Before(start) || !visit.VisitedAt.Before(end) {
			continue
		}
		review.TotalVisits++
		review.MonthlyVisits[visit.VisitedAt.In(apptime.EasternLocation()).Month()-1]++
		entry := byRestaurant[visit.Restaurant.ID]
		if entry == nil {
			entry = &restaurantYear{name: visit.Restaurant.Name}
			byRestaurant[visit.Restaurant.ID] = entry
		}
		entry.visits++
		if at := visit.VisitedAt.UnixNano(); at > entry.lastAt {
			entry.lastAt = at
		}
		if !visit.CountsInAggregates() {
			continue
		}
		entry.countedVisit++
		var pick *pickYear
		if visit.Picker != nil {
			key := pickKey{visit.Picker.ID, visit.Restaurant.ID}
			pick = picks[key]
			if pick == nil {
				pick = &pickYear{restaurant: visit.Restaurant.Name}
				picks[key] = pick
			}
		}
		for _, rating := range visit.Ratings {
			entry.ratings = append(entry.ratings, rating.Score)
			if pick != nil {
				pick.ratings = append(pick.ratings, rating.Score)
			}
		}
	}

	startNano := start.UnixNano()
	var mostVisitedAt int64
	var highestRatings int
	for id, entry := range byRestaurant {
		if firstVisit[id] >= startNano {
			review.NewRestaurants++
		} else {
			review.RepeatRestaurants++
		}
		if entry.visits >= 2 {
			current := review.MostVisited
			if current == nil || entry.visits > current.VisitCount ||
				(entry.visits == current.VisitCount && (entry.lastAt > mostVisitedAt ||
					(entry.lastAt == mostVisitedAt && (entry.name < current.Name ||
						(entry.name == current.Name && id.String() < current.ID.String()))))) {
				review.MostVisited = &model.YearRestaurantStat{ID: id, Name: entry.name, VisitCount: entry.visits}
				mostVisitedAt = entry.lastAt
			}
		}
		if len(entry.ratings) > 0 {
			avg := average(entry.ratings)
			current := review.HighestRated
			if current == nil || avg > current.AverageRating ||
				(avg == current.AverageRating && (len(entry.ratings) > highestRatings ||
					(len(entry.ratings) == highestRatings && (entry.name < current.Name ||
						(entry.name == current.Name && id.String() < current.ID.String()))))) {
				review.HighestRated = &model.YearRestaurantStat{ID: id, Name: entry.name, VisitCount: entry.countedVisit, AverageRating: avg}
				highestRatings = len(entry.ratings)
			}
		}
	}

	for _, person := range m.people {
		type candidate struct {
			id      uuid.UUID
			name    string
			average float64
			count   int
		}
		var best *candidate
		for key, pick := range picks {
			if key.person != person.ID || len(pick.ratings) == 0 {
				continue
			}
			next := candidate{id: key.restaurant, name: pick.restaurant, average: average(pick.ratings), count: len(pick.ratings)}
			if best == nil || next.average > best.average ||
				(next.average == best.average && (next.count > best.count ||
					(next.count == best.count && (next.name < best.name ||
						(next.name == best.name && next.id.String() < best.id.String()))))) {
				best = &next
			}
		}
		if best != nil {
			review.TopPicks = append(review.TopPicks, model.PersonTopPick{
				Person:        person.Name,
				RestaurantID:  best.id,
				Restaurant:    best.name,
				AverageRating: best.average,
			})
		}
	}
	return review, nil
}
