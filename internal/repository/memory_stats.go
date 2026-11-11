package repository

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

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
	ratingsByRestaurantID := map[uuid.UUID]*restaurantRatingAggregate{}
	ratingsByCuisineRestaurantID := map[string]map[uuid.UUID]*restaurantRatingAggregate{}
	ratingByPicker := map[string][]float64{}
	for _, visit := range m.visits {
		visitedRestaurants[visit.Restaurant.ID] = struct{}{}
		city := restaurantCities[visit.Restaurant.ID]
		if city == "" {
			city = strings.TrimSpace(valueOrEmpty(visit.Restaurant.City))
		}
		if city != "" {
			cities[strings.ToLower(city)] = struct{}{}
		}
		// Like Postgres, rankings count only the visits whose ratings count.
		if !visit.CountsInAggregates() {
			continue
		}
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
			restaurantAggregate.ratings = append(restaurantAggregate.ratings, rating.Score)
			if cuisineAggregate != nil {
				cuisineAggregate.ratings = append(cuisineAggregate.ratings, rating.Score)
			}
			// Everybody visits have no picker and count toward no one's average.
			if visit.Picker != nil {
				ratingByPicker[visit.Picker.Name] = append(ratingByPicker[visit.Picker.Name], rating.Score)
			}
		}
	}
	if count > 0 {
		stats.AverageRating = sum / float64(count)
	}
	stats.BestPicker, stats.BestPickerAverage = topAverageWithScore(ratingByPicker)
	stats.WorstPicker, stats.WorstPickerAverage = bottomAverageWithScore(ratingByPicker)
	stats.NewPlaces = len(visitedRestaurants)
	stats.CitiesExplored = len(cities)
	stats.TopRestaurants = topRestaurantStats(ratingsByRestaurantID)
	stats.TopRestaurantsByCuisine = topRestaurantStatsByCuisine(ratingsByCuisineRestaurantID)
	stats.BiggestSplitRestaurant, stats.BiggestSplit = biggestRatingSplit(m.visits)
	stats.MostLoyalRestaurant, stats.MostLoyalVisits, stats.NostalgiaRestaurant, stats.NostalgiaLastVisitedAt = restaurantVisitRecords(m.visits)
	stats.TasteProfiles = tasteProfiles(m.people, m.visits)
	return stats, nil
}

// biggestRatingSplit mirrors Postgres: the visit whose highest and lowest
// ratings are furthest apart, newest first and then by name on ties.
func biggestRatingSplit(visits []model.Visit) (string, float64) {
	var name string
	var split float64
	var visitedAt time.Time
	for _, visit := range visits {
		if len(visit.Ratings) < 2 {
			continue
		}
		low, high := visit.Ratings[0].Score, visit.Ratings[0].Score
		for _, rating := range visit.Ratings[1:] {
			low = math.Min(low, rating.Score)
			high = math.Max(high, rating.Score)
		}
		gap := high - low
		if gap <= 0 {
			continue
		}
		if name == "" || gap > split ||
			(gap == split && (visit.VisitedAt.After(visitedAt) ||
				(visit.VisitedAt.Equal(visitedAt) && visit.Restaurant.Name < name))) {
			name, split, visitedAt = visit.Restaurant.Name, gap, visit.VisitedAt
		}
	}
	return name, split
}

type restaurantVisitRecord struct {
	name          string
	visitCount    int
	lastVisitedAt time.Time
}

// restaurantVisitRecords mirrors Postgres for the most loyal restaurant (most
// visits, at least two) and the nostalgia restaurant (oldest last visit,
// excluding the most recently visited).
func restaurantVisitRecords(visits []model.Visit) (loyal string, loyalVisits int, nostalgia string, nostalgiaLastVisitedAt time.Time) {
	byRestaurant := map[uuid.UUID]*restaurantVisitRecord{}
	var latest time.Time
	for _, visit := range visits {
		record := byRestaurant[visit.Restaurant.ID]
		if record == nil {
			record = &restaurantVisitRecord{name: visit.Restaurant.Name}
			byRestaurant[visit.Restaurant.ID] = record
		}
		record.visitCount++
		if visit.VisitedAt.After(record.lastVisitedAt) {
			record.lastVisitedAt = visit.VisitedAt
		}
		if visit.VisitedAt.After(latest) {
			latest = visit.VisitedAt
		}
	}
	var loyalAt time.Time
	for _, record := range byRestaurant {
		if record.visitCount >= 2 && (loyal == "" || record.visitCount > loyalVisits ||
			(record.visitCount == loyalVisits && (record.lastVisitedAt.After(loyalAt) ||
				(record.lastVisitedAt.Equal(loyalAt) && record.name < loyal)))) {
			loyal, loyalVisits, loyalAt = record.name, record.visitCount, record.lastVisitedAt
		}
		if record.lastVisitedAt.Before(latest) && (nostalgia == "" || record.lastVisitedAt.Before(nostalgiaLastVisitedAt) ||
			(record.lastVisitedAt.Equal(nostalgiaLastVisitedAt) && record.name < nostalgia)) {
			nostalgia, nostalgiaLastVisitedAt = record.name, record.lastVisitedAt
		}
	}
	return loyal, loyalVisits, nostalgia, nostalgiaLastVisitedAt
}

// tasteProfiles mirrors Postgres: each person's highest-rated category on
// average over the visits whose ratings count, in people order.
func tasteProfiles(people []model.Person, visits []model.Visit) []model.PersonTasteStat {
	type categoryRatings struct {
		category string
		scores   []float64
	}
	byPerson := map[uuid.UUID]map[string]*categoryRatings{}
	for _, visit := range visits {
		category := strings.TrimSpace(valueOrEmpty(visit.Restaurant.Category))
		if category == "" || !visit.CountsInAggregates() {
			continue
		}
		key := strings.ToLower(category)
		for _, rating := range visit.Ratings {
			categories := byPerson[rating.Person.ID]
			if categories == nil {
				categories = map[string]*categoryRatings{}
				byPerson[rating.Person.ID] = categories
			}
			entry := categories[key]
			if entry == nil {
				entry = &categoryRatings{category: category}
				categories[key] = entry
			}
			if category < entry.category {
				entry.category = category
			}
			entry.scores = append(entry.scores, rating.Score)
		}
	}
	var profiles []model.PersonTasteStat
	for _, person := range people {
		var best *model.PersonTasteStat
		for _, entry := range byPerson[person.ID] {
			candidate := model.PersonTasteStat{
				Person:        person.Name,
				Category:      entry.category,
				AverageRating: average(entry.scores),
				RatingCount:   len(entry.scores),
			}
			if best == nil || candidate.AverageRating > best.AverageRating ||
				(candidate.AverageRating == best.AverageRating && (candidate.RatingCount > best.RatingCount ||
					(candidate.RatingCount == best.RatingCount && candidate.Category < best.Category))) {
				best = &candidate
			}
		}
		if best != nil {
			profiles = append(profiles, *best)
		}
	}
	return profiles
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

func bottomAverageWithScore(values map[string][]float64) (string, float64) {
	worstName := ""
	worstAvg := 0.0
	for name, scores := range values {
		avg := average(scores)
		if worstName == "" || avg < worstAvg || (avg == worstAvg && name < worstName) {
			worstName = name
			worstAvg = avg
		}
	}
	return worstName, worstAvg
}

type restaurantRatingAggregate struct {
	cuisine    string
	name       string
	visitCount int
	ratings    []float64
}

func topRestaurantStats(values map[uuid.UUID]*restaurantRatingAggregate) []model.RestaurantRatingStat {
	return topRestaurantStatsWithMinRatings(values, 2)
}

func topRestaurantStatsWithMinRatings(values map[uuid.UUID]*restaurantRatingAggregate, minRatings int) []model.RestaurantRatingStat {
	var restaurants []model.RestaurantRatingStat
	for _, value := range values {
		if len(value.ratings) < minRatings {
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
		candidates := topRestaurantStatsWithMinRatings(cuisineRestaurants, 1)
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
