package model

import (
	"encoding/base64"
	"math"
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

func TestVisitInputValidateAcceptsZeroRating(t *testing.T) {
	err := VisitInput{
		RestaurantName: "Hank's",
		VisitedAt:      time.Now(),
		PickerID:       uuid.New(),
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{uuid.New(): 0},
	}.Validate()
	if err != nil {
		t.Fatal(err)
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

func TestVisitInputValidateAcceptsJPEGPhoto(t *testing.T) {
	err := VisitInput{
		RestaurantName: "Hank's",
		VisitedAt:      time.Now(),
		PickerID:       uuid.New(),
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{uuid.New(): 8.5},
		Photos:         []VisitPhotoInput{{DataURI: visitPhotoDataURI([]byte("jpeg"))}},
	}.Validate()
	if err != nil {
		t.Fatal(err)
	}
}

func TestVisitInputValidateRejectsInvalidPhotoData(t *testing.T) {
	err := VisitInput{
		RestaurantName: "Hank's",
		VisitedAt:      time.Now(),
		PickerID:       uuid.New(),
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{uuid.New(): 8.5},
		Photos:         []VisitPhotoInput{{DataURI: "data:image/jpeg;base64,%%%"}},
	}.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestVisitInputValidateRejectsWrongPhotoMIME(t *testing.T) {
	err := VisitInput{
		RestaurantName: "Hank's",
		VisitedAt:      time.Now(),
		PickerID:       uuid.New(),
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{uuid.New(): 8.5},
		Photos:         []VisitPhotoInput{{DataURI: "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("png"))}},
	}.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestVisitInputValidateRejectsOversizedPhoto(t *testing.T) {
	err := VisitInput{
		RestaurantName: "Hank's",
		VisitedAt:      time.Now(),
		PickerID:       uuid.New(),
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{uuid.New(): 8.5},
		Photos:         []VisitPhotoInput{{DataURI: visitPhotoDataURI(make([]byte, MaxVisitPhotoBytes+1))}},
	}.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestVisitInputValidateRejectsTooManyPhotos(t *testing.T) {
	err := VisitInput{
		RestaurantName: "Hank's",
		VisitedAt:      time.Now(),
		PickerID:       uuid.New(),
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{uuid.New(): 8.5},
		Photos:         []VisitPhotoInput{{DataURI: visitPhotoDataURI([]byte("1"))}, {DataURI: visitPhotoDataURI([]byte("2"))}, {DataURI: visitPhotoDataURI([]byte("3"))}, {DataURI: visitPhotoDataURI([]byte("4"))}, {DataURI: visitPhotoDataURI([]byte("5"))}},
	}.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestVisitInputValidateRejectsDuplicateKeptPhotoIDs(t *testing.T) {
	id := uuid.New()
	err := VisitInput{
		RestaurantID: &id,
		VisitedAt:    time.Now(),
		PickerID:     uuid.New(),
		PriceLevel:   2,
		Ratings:      map[uuid.UUID]float64{uuid.New(): 8.5},
		KeepPhotoIDs: []uuid.UUID{id, id},
	}.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRestaurantInputValidateRequiresName(t *testing.T) {
	err := RestaurantInput{}.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRestaurantInputValidateRejectsInvalidGoogleRating(t *testing.T) {
	rating := 5.5
	err := RestaurantInput{Name: "Hank's", GoogleRating: &rating}.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func visitPhotoDataURI(data []byte) string {
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)
}

func TestVisitInputValidateChecksGoogleMetadata(t *testing.T) {
	nan, high, rating := math.NaN(), 999.0, 4.5
	badPrice, price := 9, 2
	tests := []struct {
		name     string
		metadata GoogleRestaurantMetadata
		wantErr  bool
	}{
		{name: "valid", metadata: GoogleRestaurantMetadata{GoogleRating: &rating, GooglePriceLevel: &price}},
		{name: "NaN rating", metadata: GoogleRestaurantMetadata{GoogleRating: &nan}, wantErr: true},
		{name: "rating above 5", metadata: GoogleRestaurantMetadata{GoogleRating: &high}, wantErr: true},
		{name: "price level above 4", metadata: GoogleRestaurantMetadata{GooglePriceLevel: &badPrice}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VisitInput{
				RestaurantName: "Hank's",
				VisitedAt:      time.Now(),
				PickerID:       uuid.New(),
				PriceLevel:     2,
				Ratings:        map[uuid.UUID]float64{uuid.New(): 8},
				GoogleMetadata: tt.metadata,
			}.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate() error = %v, want error %v", err, tt.wantErr)
			}
		})
	}
}

func TestRestaurantInputValidateRejectsNaNGoogleRating(t *testing.T) {
	nan := math.NaN()
	if err := (RestaurantInput{Name: "Hank's", GoogleRating: &nan}).Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}
