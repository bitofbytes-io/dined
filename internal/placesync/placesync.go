package placesync

import (
	"context"
	"strings"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
)

type DetailsClient interface {
	Configured() bool
	Details(context.Context, string) (*places.Place, error)
}

func EnrichVisitInput(ctx context.Context, client DetailsClient, input model.VisitInput) (model.VisitInput, error) {
	if client == nil || !client.Configured() {
		return input, nil
	}
	placeID := strings.TrimSpace(input.GooglePlaceID)
	if placeID == "" {
		return input, nil
	}
	place, err := client.Details(ctx, placeID)
	if err != nil {
		return input, err
	}
	if place == nil {
		return input, nil
	}
	return applyPlaceDetails(input, *place), nil
}

func MetadataFromPlace(place places.Place) model.GoogleRestaurantMetadata {
	var metadata model.GoogleRestaurantMetadata
	if place.Location.Latitude != 0 || place.Location.Longitude != 0 {
		metadata.Latitude = floatPtr(place.Location.Latitude)
		metadata.Longitude = floatPtr(place.Location.Longitude)
	}
	metadata.Phone = strings.TrimSpace(place.Phone)
	metadata.Website = strings.TrimSpace(place.Website)
	if place.Rating > 0 {
		metadata.GoogleRating = floatPtr(place.Rating)
	}
	if priceLevel := places.PriceLevelNumber(place.PriceLevel); priceLevel > 0 {
		metadata.GooglePriceLevel = intPtr(priceLevel)
	}
	return metadata
}

func applyPlaceDetails(input model.VisitInput, place places.Place) model.VisitInput {
	if strings.TrimSpace(input.GooglePlaceID) == "" {
		input.GooglePlaceID = place.ID
	}
	if strings.TrimSpace(input.RestaurantName) == "" {
		input.RestaurantName = strings.TrimSpace(place.DisplayName.Text)
	}
	if strings.TrimSpace(input.Address) == "" {
		input.Address = strings.TrimSpace(place.Address)
	}
	if strings.TrimSpace(input.City) == "" {
		input.City = places.City(place)
	}
	if strings.TrimSpace(input.Category) == "" {
		input.Category = places.Category(place)
	}

	metadata := input.GoogleMetadata
	placeMetadata := MetadataFromPlace(place)
	if placeMetadata.Latitude != nil {
		metadata.Latitude = placeMetadata.Latitude
	}
	if placeMetadata.Longitude != nil {
		metadata.Longitude = placeMetadata.Longitude
	}
	if placeMetadata.Phone != "" {
		metadata.Phone = placeMetadata.Phone
	}
	if placeMetadata.Website != "" {
		metadata.Website = placeMetadata.Website
	}
	if placeMetadata.GoogleRating != nil {
		metadata.GoogleRating = placeMetadata.GoogleRating
	}
	if placeMetadata.GooglePriceLevel != nil {
		metadata.GooglePriceLevel = placeMetadata.GooglePriceLevel
		if input.PriceLevel == 0 {
			input.PriceLevel = *placeMetadata.GooglePriceLevel
		}
	}
	input.GoogleMetadata = metadata
	return input
}

func floatPtr(value float64) *float64 {
	return &value
}

func intPtr(value int) *int {
	return &value
}
