package model

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestVisitInputValidateRequiresRating(t *testing.T) {
	err := VisitInput{
		RestaurantName: "Hank's",
		VisitedAt:      time.Now(),
		PickerID:       uuid.New(),
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{},
	}.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestVisitInputValidateAcceptsHalfPoint(t *testing.T) {
	err := VisitInput{
		RestaurantName: "Hank's",
		VisitedAt:      time.Now(),
		PickerID:       uuid.New(),
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{uuid.New(): 8.5},
	}.Validate()
	if err != nil {
		t.Fatal(err)
	}
}

func TestVisitInputValidateRejectsFiveDollarPrice(t *testing.T) {
	err := VisitInput{
		RestaurantName: "Hank's",
		VisitedAt:      time.Now(),
		PickerID:       uuid.New(),
		PriceLevel:     5,
		Ratings:        map[uuid.UUID]float64{uuid.New(): 8.5},
	}.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
}
