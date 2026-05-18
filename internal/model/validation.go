package model

import (
	"errors"
	"strings"
	"time"
)

func (in VisitInput) Validate() error {
	if in.RestaurantID == nil && strings.TrimSpace(in.RestaurantName) == "" {
		return errors.New("restaurant is required")
	}
	if in.VisitedAt.IsZero() {
		return errors.New("visit date is required")
	}
	if in.VisitedAt.After(time.Now().Add(24 * time.Hour)) {
		return errors.New("visit date is too far in the future")
	}
	if in.PickerID.String() == "00000000-0000-0000-0000-000000000000" {
		return errors.New("picker is required")
	}
	if in.PriceLevel < 1 || in.PriceLevel > 4 {
		return errors.New("price level must be between 1 and 4")
	}
	validRatings := 0
	for _, rating := range in.Ratings {
		if rating < 0 || rating > 10 || rating*2 != float64(int(rating*2)) {
			return errors.New("ratings must use 0.5 increments between 0 and 10")
		}
		validRatings++
	}
	if validRatings == 0 {
		return errors.New("at least one rating is required")
	}
	return nil
}

func (in RestaurantInput) Validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("restaurant name is required")
	}
	if in.GoogleRating != nil && (*in.GoogleRating < 0 || *in.GoogleRating > 5) {
		return errors.New("Google rating must be between 0 and 5")
	}
	if in.GooglePriceLevel != nil && (*in.GooglePriceLevel < 1 || *in.GooglePriceLevel > 4) {
		return errors.New("Google price level must be between 1 and 4")
	}
	return nil
}
