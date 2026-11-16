package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

func TestRenderDefaultsNowLocalToEasternTime(t *testing.T) {
	before := apptime.FormatDatetimeLocal(time.Now())
	var out strings.Builder
	if err := Render(&out, "log", PageData{}); err != nil {
		t.Fatal(err)
	}
	after := apptime.FormatDatetimeLocal(time.Now())

	rendered := out.String()
	if !strings.Contains(rendered, `value="`+before+`"`) && !strings.Contains(rendered, `value="`+after+`"`) {
		t.Fatalf("rendered HTML did not include Eastern datetime-local default %q or %q:\n%s", before, after, rendered)
	}
	if strings.Contains(rendered, "data-default-local-now") {
		t.Fatalf("rendered HTML still includes browser-local datetime override hook:\n%s", rendered)
	}
}

func TestRenderLogPickerOffersEverybodyAndEachPerson(t *testing.T) {
	danielID := uuid.New()
	jenID := uuid.New()
	var out strings.Builder
	err := Render(&out, "log", PageData{
		People: []model.Person{
			{ID: danielID, Name: "Daniel"},
			{ID: jenID, Name: "Jen"},
		},
		PrefillPickerID: EverybodyPickerValue,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, fragment := range []string{
		`<fieldset class="picker-field">`,
		`name="picker_id" value="everybody" checked>`,
		`name="picker_id" value="` + danielID.String() + `" >`,
		`name="picker_id" value="` + jenID.String() + `" >`,
		`<span class="chip-name">Everybody</span>`,
		`<span class="chip-name">Jen</span>`,
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("rendered log missing %q:\n%s", fragment, got)
		}
	}
	if strings.Count(got, `name="picker_id"`) != 3 || strings.Count(got, " checked>") != 1 {
		t.Fatalf("rendered log should offer three picker radios with only Everybody checked:\n%s", got)
	}
	if strings.Contains(got, `<select name="picker_id"`) {
		t.Fatalf("rendered log still uses a picker select:\n%s", got)
	}
}

func TestRenderLogPreservesPrefillCity(t *testing.T) {
	var out strings.Builder
	err := Render(&out, "log", PageData{PrefillCity: "Apex"})
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, `name="city" value="Apex"`) {
		t.Fatalf("rendered log missing city hidden input:\n%s", got)
	}
	script := appScript(t)
	if strings.Contains(script, `if (city) city.value = "";`) {
		t.Fatal("app.js clears hidden city on unmatched datalist input")
	}
	if !strings.Contains(script, `if (city) city.value = option.dataset.city || "";`) {
		t.Fatal("app.js does not overwrite city on matched datalist input")
	}
}

func TestRenderLogCarriesGoogleMetadataPrefill(t *testing.T) {
	var out strings.Builder
	err := Render(&out, "log", PageData{
		PrefillPhone:            "919-555-0100",
		PrefillWebsite:          "https://example.com",
		PrefillGoogleRating:     "4.7",
		PrefillGooglePriceLevel: "3",
		PrefillLatitude:         "35.779600",
		PrefillLongitude:        "-78.638200",
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{
		`name="phone" value="919-555-0100"`,
		`name="website" value="https://example.com"`,
		`name="google_rating" value="4.7"`,
		`name="google_price_level" value="3"`,
		`name="latitude" value="35.779600"`,
		`name="longitude" value="-78.638200"`,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered log missing %q:\n%s", fragment, rendered)
		}
	}
}

func TestRenderLogIncludesMappedCategoryOptions(t *testing.T) {
	for _, category := range []string{"Southern", "Korean", "Cuban"} {
		t.Run(category, func(t *testing.T) {
			var out strings.Builder
			err := Render(&out, "log", PageData{PrefillCategory: category})
			if err != nil {
				t.Fatal(err)
			}
			rendered := out.String()
			want := fmt.Sprintf(`<option selected>%s</option>`, category)
			if !strings.Contains(rendered, want) {
				t.Fatalf("rendered log missing selected %s category:\n%s", category, rendered)
			}
		})
	}
}

func TestRenderLogValidationAndPreservedState(t *testing.T) {
	personID := uuid.New()
	tagID := uuid.New()
	var out strings.Builder
	err := Render(&out, "log", PageData{
		People: []model.Person{
			{ID: personID, Name: "Jen"},
		},
		Tags:                []model.Tag{{ID: tagID, Name: "Great Service"}},
		NowLocal:            "2026-05-17T20:50",
		PrefillName:         "Tupelo Honey",
		PrefillAddress:      "123 Main St",
		PrefillCity:         "Apex",
		PrefillPlaceID:      "place-1",
		PrefillCategory:     "American",
		PrefillPriceLevel:   3,
		PrefillPickerID:     personID.String(),
		PrefillRestaurantID: "restaurant-1",
		PrefillNotes:        "Dinner notes",
		PrefillNewTag:       "Patio",
		PrefillIsChain:      true,
		PrefillRatings:      map[string]string{personID.String(): "8.5"},
		PrefillTagIDs:       map[string]bool{tagID.String(): true},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{
		`data-log-form`,
		`name="restaurant_name" list="restaurant-options" placeholder="Search or add restaurant" value="Tupelo Honey" required`,
		`name="city" value="Apex"`,
		`name="google_place_id" placeholder="Optional" value="place-1"`,
		`<option selected>American</option>`,
		`name="visited_at" value="2026-05-17T20:50" required`,
		`name="picker_id" value="` + personID.String() + `" checked>`,
		`value="3" selected>$$$</option>`,
		`data-rating-message aria-live="polite" hidden`,
		`min="0" max="10" step="0.5"`,
		`value="8.5" data-half-step-rating`,
		`name="tag_id" value="` + tagID.String() + `" checked`,
		`name="new_tag" placeholder="Great fries" value="Patio"`,
		`Dinner notes</textarea>`,
		`name="is_chain" value="true" checked`,
		`data-log-submit`,
		`<script src="/static/app.js"></script>`,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered log missing %q:\n%s", fragment, rendered)
		}
	}
}

func TestRenderLogShowsRatingRequirementWhenNoRating(t *testing.T) {
	var out strings.Builder
	err := Render(&out, "log", PageData{People: []model.Person{{ID: uuid.New(), Name: "Daniel"}}})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "At least one rating is required before saving.") {
		t.Fatalf("rendered log missing rating requirement:\n%s", rendered)
	}
	if strings.Contains(rendered, `data-rating-message aria-live="polite" hidden`) {
		t.Fatalf("rendered log hid rating requirement without a rating:\n%s", rendered)
	}
}

func TestRenderLogIncludesPhotoUploadAndPrefilledPhotos(t *testing.T) {
	var out strings.Builder
	err := Render(&out, "log", PageData{
		PrefillPhotoDataURIs: []string{testPhotoDataURI},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{
		`data-photo-uploader data-photo-limit="4" data-photo-max-bytes="512000"`,
		`Food, fun, memories. Up to 4.`,
		`name="photo_data_uri" value="` + strings.ReplaceAll(testPhotoDataURI, "+", "&#43;") + `"`,
		`type="file" accept="image/*" multiple data-photo-input`,
		`id="photo-preview-modal"`,
		`<script src="/static/app.js"></script>`,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered log missing %q:\n%s", fragment, rendered)
		}
	}
	addIndex := strings.Index(rendered, `data-photo-add`)
	photoIndex := strings.Index(rendered, `name="photo_data_uri" value="`+strings.ReplaceAll(testPhotoDataURI, "+", "&#43;")+`"`)
	if addIndex < 0 || photoIndex < 0 || addIndex > photoIndex {
		t.Fatalf("rendered log should place add-photo tile before photo thumbnails:\n%s", rendered)
	}
}
