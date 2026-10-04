package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Person struct {
	ID          uuid.UUID
	Name        string
	AvatarColor string
}

type Restaurant struct {
	ID               uuid.UUID
	Name             string
	Address          *string
	City             *string
	Latitude         *float64
	Longitude        *float64
	Phone            *string
	Website          *string
	GooglePlaceID    *string
	GoogleRating     *float64
	GooglePriceLevel *int
	Category         *string
	IsChain          bool
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type RestaurantMapPoint struct {
	RestaurantID    uuid.UUID
	Name            string
	Latitude        float64
	Longitude       float64
	VisitCount      int
	LatestVisitedAt time.Time
}

type GoogleRestaurantMetadata struct {
	Latitude         *float64
	Longitude        *float64
	Phone            string
	Website          string
	GoogleRating     *float64
	GooglePriceLevel *int
	Category         string
}

// MinAggregateRaters is how many people must rate a visit before its ratings
// count in averages and rankings across visits.
const MinAggregateRaters = 2

// ErrUnknownPicker means a visit names a picker who is not one of the people.
var ErrUnknownPicker = errors.New("picker not found")

// PlaceConflictError means a visit chose an existing restaurant but submitted
// a Google place ID that is not that restaurant's: another restaurant owns it
// (Owner is that restaurant's name), or the chosen restaurant is already
// linked to a different place (Owner is empty). The visit is not saved.
type PlaceConflictError struct {
	Restaurant string
	Owner      string
}

func (e *PlaceConflictError) Error() string {
	if e.Owner != "" {
		return fmt.Sprintf("google place id belongs to %q, not chosen restaurant %q", e.Owner, e.Restaurant)
	}
	return fmt.Sprintf("chosen restaurant %q is linked to a different google place", e.Restaurant)
}

type Visit struct {
	ID         uuid.UUID
	Restaurant Restaurant
	VisitedAt  time.Time
	// Picker is the person who chose the restaurant, or nil when Everybody did.
	Picker     *Person
	PriceLevel int
	Notes      *string
	Ratings    []Rating
	Tags       []Tag
	Photos     []VisitPhoto
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// CountsInAggregates reports whether enough people rated the visit for its
// ratings to count in averages and rankings across visits.
func (v Visit) CountsInAggregates() bool {
	return len(v.Ratings) >= MinAggregateRaters
}

type VisitPhoto struct {
	ID          uuid.UUID
	VisitID     uuid.UUID
	DataURI     string
	ContentType string
	ByteCount   int
	SortOrder   int
	CreatedAt   time.Time
}

type Rating struct {
	Person Person
	Score  float64
}

type Tag struct {
	ID   uuid.UUID
	Name string
}

type VisitInput struct {
	RestaurantID   *uuid.UUID
	RestaurantName string
	Address        string
	City           string
	GooglePlaceID  string
	GoogleMetadata GoogleRestaurantMetadata
	Category       string
	IsChain        bool
	VisitedAt      time.Time
	// PickerID is the person who chose the restaurant, or nil for Everybody.
	PickerID     *uuid.UUID
	PriceLevel   int
	Notes        string
	Ratings      map[uuid.UUID]float64
	TagIDs       []uuid.UUID
	NewTag       string
	KeepPhotoIDs []uuid.UUID
	Photos       []VisitPhotoInput
}

type VisitPhotoInput struct {
	DataURI string
}

type RestaurantInput struct {
	Name             string
	Address          string
	City             string
	Phone            string
	Website          string
	GoogleRating     *float64
	GooglePriceLevel *int
	Category         string
	IsChain          bool
}

type Stats struct {
	TotalDines              int
	AverageRating           float64
	BestPicker              string
	BestPickerAverage       float64
	WorstPicker             string
	WorstPickerAverage      float64
	NewPlaces               int
	CitiesExplored          int
	TopRestaurants          []RestaurantRatingStat
	TopRestaurantsByCuisine []CuisineRestaurantStat
}

type RestaurantRatingStat struct {
	Name          string
	AverageRating float64
	RatingCount   int
	VisitCount    int
}

type CuisineRestaurantStat struct {
	Cuisine       string
	Name          string
	AverageRating float64
	RatingCount   int
	VisitCount    int
}
