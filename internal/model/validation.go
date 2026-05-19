package model

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
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
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0, errors.New("photo data is required")
	}
	if !strings.HasPrefix(strings.ToLower(trimmed), visitPhotoPrefix) {
		return 0, errors.New("dine photos must be JPEG images")
	}
	encoded := trimmed[len(visitPhotoPrefix):]
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return 0, errors.New("dine photos must contain valid base64 image data")
	}
	if len(decoded) > MaxVisitPhotoBytes {
		return 0, fmt.Errorf("dine photos must be smaller than %dKB", MaxVisitPhotoBytes/1024)
	}
	return len(decoded), nil
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
