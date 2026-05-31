package model

import (
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

type Visit struct {
	ID         uuid.UUID
	Restaurant Restaurant
	VisitedAt  time.Time
	Picker     Person
	PriceLevel int
	Notes      *string
	Ratings    []Rating
	Tags       []Tag
	Photos     []VisitPhoto
	CreatedAt  time.Time
	UpdatedAt  time.Time
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
	PickerID       uuid.UUID
	PriceLevel     int
	Notes          string
	Ratings        map[uuid.UUID]float64
	TagIDs         []uuid.UUID
	NewTag         string
	KeepPhotoIDs   []uuid.UUID
	Photos         []VisitPhotoInput
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
	MostVisitedRestaurant   string
	HighestRatedRestaurant  string
	BestPicker              string
	BestPickerAverage       float64
	WorstPicker             string
	WorstPickerAverage      float64
	BiggestSplitRestaurant  string
	NewPlaces               int
	CitiesExplored          int
	TopRestaurants          []RestaurantRatingStat
	TopRestaurantsByCuisine []CuisineRestaurantStat
}

type PickerTurn struct {
	LastPicker Person
	NextPicker Person
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
