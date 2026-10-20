package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	MaxVisitPhotos     = 4
	MaxVisitPhotoBytes = 500 * 1024
	visitPhotoPrefix   = "data:image/jpeg;base64,"
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
	if err := validateGoogleRatingAndPriceLevel(in.GoogleMetadata.GoogleRating, in.GoogleMetadata.GooglePriceLevel); err != nil {
		return err
	}
	if len(in.KeepPhotoIDs)+len(in.Photos) > MaxVisitPhotos {
		return fmt.Errorf("dine photos must be %d or fewer", MaxVisitPhotos)
	}
	seenPhotoIDs := map[string]bool{}
	for _, id := range in.KeepPhotoIDs {
		if id.String() == "00000000-0000-0000-0000-000000000000" {
			return errors.New("photo id is required")
		}
		if seenPhotoIDs[id.String()] {
			return errors.New("photo ids must be unique")
		}
		seenPhotoIDs[id.String()] = true
	}
	for _, photo := range in.Photos {
		if _, err := ValidateVisitPhotoDataURI(photo.DataURI); err != nil {
			return err
		}
	}
	return nil
}

func ValidateVisitPhotoDataURI(raw string) (int, error) {
	decoded, err := DecodeVisitPhotoDataURI(raw)
	if err != nil {
		return 0, err
	}
	return len(decoded), nil
}

func (in RestaurantInput) Validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("restaurant name is required")
	}
	return validateGoogleRatingAndPriceLevel(in.GoogleRating, in.GooglePriceLevel)
}

// MaxWantToTryNoteLength caps a Want to Try note, in characters.
const MaxWantToTryNoteLength = 500

func (in WantToTryInput) Validate() error {
	if in.RestaurantID == nil && strings.TrimSpace(in.RestaurantName) == "" {
		return errors.New("restaurant is required")
	}
	if utf8.RuneCountInString(strings.TrimSpace(in.Note)) > MaxWantToTryNoteLength {
		return fmt.Errorf("note must be %d characters or fewer", MaxWantToTryNoteLength)
	}
	return validateGoogleRatingAndPriceLevel(in.GoogleMetadata.GoogleRating, in.GoogleMetadata.GooglePriceLevel)
}

// VisitInput returns the restaurant fields of a Want to Try input as a visit
// input, so saving finds or creates the restaurant the same way a dine does.
func (in WantToTryInput) VisitInput() VisitInput {
	return VisitInput{
		RestaurantID:   in.RestaurantID,
		RestaurantName: in.RestaurantName,
		Address:        in.Address,
		City:           in.City,
		GooglePlaceID:  in.GooglePlaceID,
		GoogleMetadata: in.GoogleMetadata,
		Category:       in.Category,
	}
}

func validateGoogleRatingAndPriceLevel(rating *float64, priceLevel *int) error {
	// Written so that NaN, which fails every comparison, is rejected too.
	if rating != nil && !(*rating >= 0 && *rating <= 5) {
		return errors.New("rating from Google must be between 0 and 5")
	}
	if priceLevel != nil && (*priceLevel < 1 || *priceLevel > 4) {
		return errors.New("price level from Google must be between 1 and 4")
	}
	return nil
}
