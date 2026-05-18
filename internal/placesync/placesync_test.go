package placesync

import (
	"context"
	"errors"
	"testing"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
)

func TestEnrichVisitInputAppliesPlaceDetails(t *testing.T) {
	client := &fakeDetailsClient{
		configured: true,
		places: map[string]places.Place{
			"place-1": tupeloPlace("place-1"),
		},
	}
	input := model.VisitInput{GooglePlaceID: "place-1"}

	got, err := EnrichVisitInput(context.Background(), client, input)
	if err != nil {
		t.Fatal(err)
	}
	if got.RestaurantName != "Tupelo Honey Southern Kitchen & Bar" {
		t.Fatalf("RestaurantName = %q", got.RestaurantName)
	}
	if got.Address != "425 Oberlin Rd, Raleigh, NC 27605, USA" {
		t.Fatalf("Address = %q", got.Address)
	}
	if got.City != "Raleigh" {
		t.Fatalf("City = %q", got.City)
	}
	if got.Category != "American" {
		t.Fatalf("Category = %q", got.Category)
	}
	if got.PriceLevel != 2 {
		t.Fatalf("PriceLevel = %d", got.PriceLevel)
	}
	assertGoogleMetadata(t, got.GoogleMetadata)
}

func TestEnrichVisitInputReturnsOriginalInputWhenDetailsFails(t *testing.T) {
	client := &fakeDetailsClient{
		configured: true,
		errByID:    map[string]error{"place-1": errors.New("api down")},
	}
	input := model.VisitInput{RestaurantName: "Manual", GooglePlaceID: "place-1", PriceLevel: 2}

	got, err := EnrichVisitInput(context.Background(), client, input)
	if err == nil {
		t.Fatal("expected details error")
	}
	if got.RestaurantName != input.RestaurantName || got.PriceLevel != input.PriceLevel {
		t.Fatalf("input was mutated on error: %#v", got)
	}
}

type fakeDetailsClient struct {
	configured bool
	places     map[string]places.Place
	errByID    map[string]error
}

func (f *fakeDetailsClient) Configured() bool {
	return f.configured
}

func (f *fakeDetailsClient) Details(_ context.Context, placeID string) (*places.Place, error) {
	if err := f.errByID[placeID]; err != nil {
		return nil, err
	}
	place, ok := f.places[placeID]
	if !ok {
		return nil, nil
	}
	return &place, nil
}

func tupeloPlace(id string) places.Place {
	return places.Place{
		ID:          id,
		DisplayName: places.Name{Text: "Tupelo Honey Southern Kitchen & Bar"},
		Address:     "425 Oberlin Rd, Raleigh, NC 27605, USA",
		AddressComponents: []places.AddressComponent{
			{LongText: "Raleigh", ShortText: "Raleigh", Types: []string{"locality", "political"}},
		},
		Location:   places.Location{Latitude: 35.7903375, Longitude: -78.6631725},
		Phone:      "(919) 723-9353",
		Website:    "https://tupelohoneycafe.com/restaurant/raleigh/",
		Rating:     4.3,
		PriceLevel: "PRICE_LEVEL_MODERATE",
		Types:      []string{"brunch_restaurant", "american_restaurant", "restaurant"},
	}
}

func assertGoogleMetadata(t *testing.T, metadata model.GoogleRestaurantMetadata) {
	t.Helper()
	if metadata.Latitude == nil || *metadata.Latitude != 35.7903375 {
		t.Fatalf("Latitude = %#v", metadata.Latitude)
	}
	if metadata.Longitude == nil || *metadata.Longitude != -78.6631725 {
		t.Fatalf("Longitude = %#v", metadata.Longitude)
	}
	if metadata.Phone != "(919) 723-9353" {
		t.Fatalf("Phone = %q", metadata.Phone)
	}
	if metadata.Website != "https://tupelohoneycafe.com/restaurant/raleigh/" {
		t.Fatalf("Website = %q", metadata.Website)
	}
	if metadata.GoogleRating == nil || *metadata.GoogleRating != 4.3 {
		t.Fatalf("GoogleRating = %#v", metadata.GoogleRating)
	}
	if metadata.GooglePriceLevel == nil || *metadata.GooglePriceLevel != 2 {
		t.Fatalf("GooglePriceLevel = %#v", metadata.GooglePriceLevel)
	}
}
