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

// ErrRestaurantNotFound means an input chose a restaurant that does not exist.
var ErrRestaurantNotFound = errors.New("restaurant not found")

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
	Bytes          []byte // The requested original or thumbnail representation; absent on visit reads.
	ThumbnailBytes []byte // Memory-store backing only; absent on visit reads.
	ID             uuid.UUID
	VisitID        uuid.UUID
	DataURI        string
	ContentType    string
	ByteCount      int
	SortOrder      int
	CreatedAt      time.Time
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
	DataURI  string
	prepared *preparedVisitPhoto
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

// WantToTryItem is a restaurant on the family's Want to Try list.
type WantToTryItem struct {
	ID         uuid.UUID
	Restaurant Restaurant
	// SuggestedBy is the person whose idea it was, or nil when it was Everybody's.
	SuggestedBy *Person
	Note        *string
	CreatedAt   time.Time
	// DoneAt is when a dine logged at the restaurant took it off the list.
	DoneAt *time.Time
}

// WantToTryInput saves a restaurant to the Want to Try list. Like a visit, it
// names a chosen restaurant (RestaurantID) or describes one to find or create.
type WantToTryInput struct {
	RestaurantID   *uuid.UUID
	RestaurantName string
	Address        string
	City           string
	GooglePlaceID  string
	GoogleMetadata GoogleRestaurantMetadata
	Category       string
	// SuggestedByID is the person whose idea it was, or nil for Everybody.
	SuggestedByID *uuid.UUID
	Note          string
}

// RestedFavorite is a restaurant the family rated well and has not been back
// to lately, so the "Where should we eat?" spinner can land on it.
type RestedFavorite struct {
	Restaurant Restaurant
	// AverageRating counts only visits whose ratings count in aggregates.
	AverageRating float64
	LastVisitedAt time.Time
}

// PickCount is how many times a person picked where to eat since some time.
type PickCount struct {
	Person Person
	Picks  int
	// LastPickedAt is the person's latest pick ever, or nil if they never picked.
	LastPickedAt *time.Time
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
	// BiggestSplitRestaurant is where one visit's highest and lowest ratings
	// were furthest apart; BiggestSplit is that gap.
	BiggestSplitRestaurant string
	BiggestSplit           float64
	// MostLoyalRestaurant is the restaurant with the most visits, once any
	// restaurant has been visited at least twice.
	MostLoyalRestaurant string
	MostLoyalVisits     int
	// NostalgiaRestaurant is the restaurant whose last visit is the oldest,
	// once the family has dined somewhere else since.
	NostalgiaRestaurant    string
	NostalgiaLastVisitedAt time.Time
	// TasteProfiles holds each person's favorite category, in people order.
	TasteProfiles []PersonTasteStat
}

// PersonTasteStat is the category a person rates highest on average.
type PersonTasteStat struct {
	Person        string
	Category      string
	AverageRating float64
	RatingCount   int
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
