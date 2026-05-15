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

type Visit struct {
	ID         uuid.UUID
	Restaurant Restaurant
	VisitedAt  time.Time
	Picker     Person
	PriceLevel int
	Notes      *string
	Ratings    []Rating
	Tags       []Tag
	CreatedAt  time.Time
	UpdatedAt  time.Time
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
	GooglePlaceID  string
	Category       string
	IsChain        bool
	VisitedAt      time.Time
	PickerID       uuid.UUID
	PriceLevel     int
	Notes          string
	Ratings        map[uuid.UUID]float64
	TagIDs         []uuid.UUID
	NewTag         string
}

type Stats struct {
	TotalDines             int
	AverageRating          float64
	MostVisitedRestaurant  string
	HighestRatedRestaurant string
	BestPicker             string
	BiggestSplitRestaurant string
}

type PickerTurn struct {
	LastPicker Person
	NextPicker Person
}
