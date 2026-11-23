package handler

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/ui"
	"github.com/google/uuid"
)

// visitFormValues is a valid dine form; cases override or add fields.
func visitFormValues(overrides url.Values) url.Values {
	form := url.Values{
		"restaurant_name": {"Hank's"},
		"visited_at":      {"2026-03-05T19:30"},
	}
	for key, values := range overrides {
		form[key] = values
	}
	return form
}

func TestParseVisitFormDate(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr bool
	}{
		{name: "datetime-local value", value: "2026-03-05T19:30", want: "2026-03-05T19:30"},
		{name: "midnight", value: "2026-12-31T00:00", want: "2026-12-31T00:00"},
		{name: "missing", value: "", wantErr: true},
		{name: "not a date", value: "yesterday", wantErr: true},
		{name: "date only", value: "2026-03-05", wantErr: true},
		{name: "seconds", value: "2026-03-05T19:30:15", wantErr: true},
		{name: "impossible day", value: "2026-02-30T10:00", wantErr: true},
		{name: "impossible hour", value: "2026-03-05T25:00", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := visitFormValues(url.Values{"visited_at": {tt.value}})
			input, err := parseVisitForm(form, form)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseVisitForm accepted visited_at %q", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := apptime.FormatDatetimeLocal(input.VisitedAt); got != tt.want {
				t.Fatalf("VisitedAt = %q, want %q", got, tt.want)
			}
			if input.VisitedAt.Location() != apptime.EasternLocation() {
				t.Fatalf("VisitedAt location = %v, want Eastern", input.VisitedAt.Location())
			}
		})
	}
}

func TestParseVisitFormPicker(t *testing.T) {
	personID := uuid.New()
	tests := []struct {
		name    string
		value   string
		want    *uuid.UUID
		wantErr error
	}{
		{name: "missing is everybody", value: ""},
		{name: "blank is everybody", value: "   "},
		{name: "everybody", value: ui.EverybodyPickerValue},
		{name: "person", value: personID.String(), want: &personID},
		{name: "person with whitespace", value: " " + personID.String() + " ", want: &personID},
		{name: "not a uuid", value: "dad", wantErr: errInvalidPicker},
		{name: "everybody is case sensitive", value: "Everybody", wantErr: errInvalidPicker},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := visitFormValues(url.Values{"picker_id": {tt.value}})
			input, err := parseVisitForm(form, form)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case tt.want == nil && input.PickerID != nil:
				t.Fatalf("PickerID = %v, want nil", *input.PickerID)
			case tt.want != nil && (input.PickerID == nil || *input.PickerID != *tt.want):
				t.Fatalf("PickerID = %v, want %v", input.PickerID, *tt.want)
			}
		})
	}
}

func TestPickerLogValue(t *testing.T) {
	personID := uuid.New()
	if got := pickerLogValue(nil); got != ui.EverybodyPickerValue {
		t.Fatalf("pickerLogValue(nil) = %q, want %q", got, ui.EverybodyPickerValue)
	}
	if got := pickerLogValue(&personID); got != personID.String() {
		t.Fatalf("pickerLogValue(id) = %q, want %q", got, personID.String())
	}
}

func TestParseVisitFormPriceLevel(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int
	}{
		{name: "missing", value: "", want: 0},
		{name: "one", value: "1", want: 1},
		{name: "four", value: "4", want: 4},
		// An unreadable price level is dropped, not rejected, so a dine can be
		// saved without one; the model validates the range.
		{name: "not a number", value: "$$", want: 0},
		{name: "decimal", value: "2.5", want: 0},
		{name: "out of range is passed through", value: "9", want: 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := visitFormValues(url.Values{"price_level": {tt.value}})
			input, err := parseVisitForm(form, form)
			if err != nil {
				t.Fatal(err)
			}
			if input.PriceLevel != tt.want {
				t.Fatalf("PriceLevel = %d, want %d", input.PriceLevel, tt.want)
			}
		})
	}
}

func TestParseVisitFormGooglePriceLevel(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    *int
		wantErr string
	}{
		{name: "missing", value: ""},
		{name: "blank", value: "  "},
		{name: "number", value: "3", want: intPtr(3)},
		{name: "padded number", value: " 2 ", want: intPtr(2)},
		{name: "not a number", value: "expensive", wantErr: "Google price level must be a number"},
		{name: "decimal", value: "1.5", wantErr: "Google price level must be a number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := visitFormValues(url.Values{"google_price_level": {tt.value}})
			input, err := parseVisitForm(form, form)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := input.GoogleMetadata.GooglePriceLevel
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("GooglePriceLevel = %d, want nil", *got)
			case tt.want != nil && (got == nil || *got != *tt.want):
				t.Fatalf("GooglePriceLevel = %v, want %d", got, *tt.want)
			}
		})
	}
}

func TestParseVisitFormGoogleRating(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    *float64
		wantErr string
	}{
		{name: "missing", value: ""},
		{name: "decimal", value: "4.7", want: floatPtr(4.7)},
		{name: "not a number", value: "great", wantErr: "Google rating must be a number"},
		{name: "not finite", value: "NaN", wantErr: "Google rating must be a finite number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := visitFormValues(url.Values{"google_rating": {tt.value}})
			input, err := parseVisitForm(form, form)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got := input.GoogleMetadata.GoogleRating
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("GoogleRating = %v, want nil", *got)
			case tt.want != nil && (got == nil || *got != *tt.want):
				t.Fatalf("GoogleRating = %v, want %v", got, *tt.want)
			}
		})
	}
}

func TestParseVisitFormRatings(t *testing.T) {
	dad, kid := uuid.New(), uuid.New()
	tests := []struct {
		name    string
		post    url.Values
		want    map[uuid.UUID]float64
		wantErr bool
	}{
		{name: "none", post: url.Values{}, want: map[uuid.UUID]float64{}},
		{
			name: "several people",
			post: url.Values{"rating_" + dad.String(): {"8"}, "rating_" + kid.String(): {"6.5"}},
			want: map[uuid.UUID]float64{dad: 8, kid: 6.5},
		},
		{
			name: "blank score is skipped",
			post: url.Values{"rating_" + dad.String(): {"8"}, "rating_" + kid.String(): {""}},
			want: map[uuid.UUID]float64{dad: 8},
		},
		{
			name: "unrelated fields are ignored",
			post: url.Values{"rating": {"9"}, "rate_" + dad.String(): {"9"}},
			want: map[uuid.UUID]float64{},
		},
		{name: "person is not a uuid", post: url.Values{"rating_dad": {"8"}}, wantErr: true},
		{name: "bare prefix", post: url.Values{"rating_": {"8"}}, wantErr: true},
		{name: "score is not a number", post: url.Values{"rating_" + dad.String(): {"great"}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := visitFormValues(nil)
			input, err := parseVisitForm(form, tt.post)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(input.Ratings) != len(tt.want) {
				t.Fatalf("Ratings = %v, want %v", input.Ratings, tt.want)
			}
			for id, score := range tt.want {
				if got, ok := input.Ratings[id]; !ok || got != score {
					t.Fatalf("Ratings[%v] = %v (present %v), want %v", id, got, ok, score)
				}
			}
		})
	}
}

func TestParseVisitFormTags(t *testing.T) {
	first, second := uuid.New(), uuid.New()
	tests := []struct {
		name    string
		post    url.Values
		want    []uuid.UUID
		wantErr bool
	}{
		{name: "none", post: url.Values{}},
		{name: "one", post: url.Values{"tag_id": {first.String()}}, want: []uuid.UUID{first}},
		{name: "several keep their order", post: url.Values{"tag_id": {second.String(), first.String()}}, want: []uuid.UUID{second, first}},
		{name: "not a uuid", post: url.Values{"tag_id": {first.String(), "spicy"}}, wantErr: true},
		{name: "blank", post: url.Values{"tag_id": {""}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input, err := parseVisitForm(visitFormValues(nil), tt.post)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(input.TagIDs) != len(tt.want) {
				t.Fatalf("TagIDs = %v, want %v", input.TagIDs, tt.want)
			}
			for i := range tt.want {
				if input.TagIDs[i] != tt.want[i] {
					t.Fatalf("TagIDs = %v, want %v", input.TagIDs, tt.want)
				}
			}
		})
	}
}

func TestParseVisitFormRestaurantAndPhotos(t *testing.T) {
	restaurantID, photoID := uuid.New(), uuid.New()

	input, err := parseVisitForm(visitFormValues(url.Values{"restaurant_id": {restaurantID.String()}}), url.Values{
		"keep_photo_id":  {photoID.String()},
		"photo_data_uri": {"data:image/png;base64,AAAA", "  ", ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.RestaurantID == nil || *input.RestaurantID != restaurantID {
		t.Fatalf("RestaurantID = %v, want %v", input.RestaurantID, restaurantID)
	}
	if len(input.KeepPhotoIDs) != 1 || input.KeepPhotoIDs[0] != photoID {
		t.Fatalf("KeepPhotoIDs = %v", input.KeepPhotoIDs)
	}
	if len(input.Photos) != 1 || input.Photos[0].DataURI != "data:image/png;base64,AAAA" {
		t.Fatalf("Photos = %v, want only the non-blank upload", input.Photos)
	}

	for name, tt := range map[string]struct {
		form url.Values
		post url.Values
	}{
		"restaurant id": {form: visitFormValues(url.Values{"restaurant_id": {"hanks"}}), post: url.Values{}},
		"kept photo id": {form: visitFormValues(nil), post: url.Values{"keep_photo_id": {"photo-1"}}},
	} {
		if _, err := parseVisitForm(tt.form, tt.post); err == nil {
			t.Fatalf("%s: expected an error for a malformed id", name)
		}
	}
}

func TestParseVisitFormFlagsAndText(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "true", value: "true", want: true},
		{name: "false", value: "false"},
		{name: "on is not true", value: "on"},
		{name: "missing", value: ""},
	}
	for _, tt := range tests {
		t.Run("is_chain "+tt.name, func(t *testing.T) {
			form := visitFormValues(url.Values{"is_chain": {tt.value}})
			input, err := parseVisitForm(form, form)
			if err != nil {
				t.Fatal(err)
			}
			if input.IsChain != tt.want {
				t.Fatalf("IsChain = %v, want %v", input.IsChain, tt.want)
			}
		})
	}

	form := visitFormValues(url.Values{
		"address":  {"1 Main St"},
		"city":     {"Raleigh"},
		"category": {"Diner"},
		"notes":    {"Great pie"},
		"new_tag":  {"pie"},
	})
	input, err := parseVisitForm(form, form)
	if err != nil {
		t.Fatal(err)
	}
	if input.RestaurantName != "Hank's" || input.Address != "1 Main St" || input.City != "Raleigh" ||
		input.Category != "Diner" || input.Notes != "Great pie" || input.NewTag != "pie" {
		t.Fatalf("text fields = %#v", input)
	}
}

func TestParseVisitFormCoordinates(t *testing.T) {
	tests := []struct {
		name          string
		latitude      string
		longitude     string
		wantLatitude  *float64
		wantLongitude *float64
		wantErr       string
	}{
		{name: "missing"},
		{name: "valid", latitude: "35.7796", longitude: "-78.6382", wantLatitude: floatPtr(35.7796), wantLongitude: floatPtr(-78.6382)},
		{name: "limits", latitude: "-90", longitude: "180", wantLatitude: floatPtr(-90), wantLongitude: floatPtr(180)},
		{name: "latitude only", latitude: "35.7796", wantLatitude: floatPtr(35.7796)},
		{name: "longitude only", longitude: "-78.6382", wantLongitude: floatPtr(-78.6382)},
		{name: "latitude too high", latitude: "90.5", wantErr: "latitude must be between -90 and 90"},
		{name: "longitude too low", longitude: "-181", wantErr: "longitude must be between -180 and 180"},
		{name: "latitude not a number", latitude: "north", wantErr: "latitude must be a number"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := visitFormValues(url.Values{"latitude": {tt.latitude}, "longitude": {tt.longitude}})
			input, err := parseVisitForm(form, form)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			assertFloatPtr(t, "Latitude", input.GoogleMetadata.Latitude, tt.wantLatitude)
			assertFloatPtr(t, "Longitude", input.GoogleMetadata.Longitude, tt.wantLongitude)
		})
	}
}

func assertFloatPtr(t *testing.T, name string, got, want *float64) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Fatalf("%s = %v, want nil", name, *got)
	case want != nil && got == nil:
		t.Fatalf("%s = nil, want %v", name, *want)
	case want != nil && *got != *want:
		t.Fatalf("%s = %v, want %v", name, *got, *want)
	}
}

func TestParseVisitFormReadsScalarsFromForm(t *testing.T) {
	// Single-value fields are read from the merged form, but ratings are only
	// read from the request body.
	personID := uuid.New()
	query := url.Values{"restaurant_name": {"From Query"}, "rating_" + personID.String(): {"7"}}
	input, err := parseVisitForm(visitFormValues(query), url.Values{})
	if err != nil {
		t.Fatal(err)
	}
	if input.RestaurantName != "From Query" {
		t.Fatalf("RestaurantName = %q", input.RestaurantName)
	}
	if len(input.Ratings) != 0 {
		t.Fatalf("Ratings = %v, want none from the query string", input.Ratings)
	}
}

func intPtr(v int) *int { return &v }

func floatPtr(v float64) *float64 { return &v }
