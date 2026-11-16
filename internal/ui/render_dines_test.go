package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

func TestRenderVisitListShowsEverybodyPicker(t *testing.T) {
	var out strings.Builder
	err := Render(&out, "dines", PageData{Visits: []model.Visit{
		{ID: uuid.New(), Restaurant: model.Restaurant{Name: "Hank's"}, Picker: &model.Person{Name: "Daniel"}, PriceLevel: 2},
		{ID: uuid.New(), Restaurant: model.Restaurant{Name: "Amigos"}, PriceLevel: 2},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"Picked by Daniel", "Picked by Everybody"} {
		if !strings.Contains(out.String(), fragment) {
			t.Fatalf("rendered dines missing %q:\n%s", fragment, out.String())
		}
	}
}

func TestRenderAuthenticatedDinesUsesEditLinkAndDeleteConfirmationModal(t *testing.T) {
	visitID := uuid.New()
	restaurantID := uuid.New()
	var out strings.Builder
	err := Render(&out, "dines", PageData{
		Authenticated: true,
		Visits: []model.Visit{{
			ID:         visitID,
			Restaurant: model.Restaurant{ID: restaurantID, Name: "Hank's"},
			VisitedAt:  time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
			Picker:     &model.Person{Name: "Daniel"},
			PriceLevel: 2,
			Ratings:    []model.Rating{{Person: model.Person{Name: "Daniel"}, Score: 8}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	rendered := out.String()
	for _, fragment := range []string{
		`/visits/` + visitID.String() + `/edit`,
		`action="/visits/` + visitID.String() + `/delete" hx-boost="false" data-delete-dine-form`,
		`onsubmit="return dinedConfirmDelete(event, this)"`,
		`data-delete-title="Delete this dine?"`,
		`data-delete-message="This removes the dine and its ratings from the ledger."`,
		`<dialog class="confirm-modal" id="delete-dine-modal"`,
		`id="delete-dine-cancel-button"`,
		`id="delete-dine-confirm-button"`,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered dines missing %q:\n%s", fragment, rendered)
		}
	}
	if strings.Contains(rendered, `onsubmit="return confirm`) {
		t.Fatalf("rendered dines still uses inline browser confirm:\n%s", rendered)
	}
}

func TestRenderDinesShowsPhotoStripAndPreviewHooks(t *testing.T) {
	visitID := uuid.New()
	restaurantID := uuid.New()
	photos := []model.VisitPhoto{{ID: uuid.New()}, {ID: uuid.New()}, {ID: uuid.New()}, {ID: uuid.New()}}
	var out strings.Builder
	err := Render(&out, "dines", PageData{
		Visits: []model.Visit{{
			ID:         visitID,
			Restaurant: model.Restaurant{ID: restaurantID, Name: "Hank's"},
			VisitedAt:  time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
			Picker:     &model.Person{Name: "Daniel"},
			PriceLevel: 2,
			Ratings:    []model.Rating{{Person: model.Person{Name: "Daniel"}, Score: 8}},
			Photos:     photos,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{
		`class="visit-photo-strip" data-photo-strip`,
		`data-photo-preview data-photo-src="/photos/` + photos[0].ID.String() + `"`,
		`<img src="/photos/` + photos[3].ID.String() + `?variant=thumb-v1" alt="" loading="lazy">`,
		`id="photo-preview-image"`,
		`id="photo-preview-prev"`,
		`id="photo-preview-next"`,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered dines missing %q:\n%s", fragment, rendered)
		}
	}
	if strings.Contains(rendered, `photo-more`) || strings.Contains(rendered, `>+1</button>`) {
		t.Fatalf("rendered dines should show all four photos instead of a more tile:\n%s", rendered)
	}
}

func TestRenderVisitEditPrefillsRatingsTagsAndNotes(t *testing.T) {
	danielID := uuid.New()
	jenID := uuid.New()
	tagID := uuid.New()
	visitID := uuid.New()
	restaurantID := uuid.New()
	photoID := uuid.New()
	visitedAt := time.Date(2026, 5, 16, 19, 30, 0, 0, apptime.EasternLocation())
	note := "Updated score after dessert."
	var out strings.Builder
	err := Render(&out, "visit-edit", PageData{
		Visit: &model.Visit{
			ID:         visitID,
			Restaurant: model.Restaurant{ID: restaurantID, Name: "Hank's"},
			VisitedAt:  visitedAt,
			Picker:     &model.Person{ID: jenID, Name: "Jen"},
			PriceLevel: 3,
			Notes:      &note,
			Ratings:    []model.Rating{{Person: model.Person{ID: danielID, Name: "Daniel"}, Score: 8.5}},
			Tags:       []model.Tag{{ID: tagID, Name: "Would Return"}},
			Photos:     []model.VisitPhoto{{ID: photoID, DataURI: testPhotoDataURI, ContentType: "image/jpeg", ByteCount: 5}},
		},
		People: []model.Person{{ID: danielID, Name: "Daniel"}, {ID: jenID, Name: "Jen"}},
		Tags:   []model.Tag{{ID: tagID, Name: "Would Return"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{
		`action="/visits/` + visitID.String() + `"`,
		`name="restaurant_id" value="` + restaurantID.String() + `"`,
		`value="` + apptime.FormatDatetimeLocal(visitedAt) + `"`,
		`name="picker_id" value="` + jenID.String() + `" checked>`,
		`name="rating_` + danielID.String() + `" type="number" min="0" max="10" step="0.5" inputmode="decimal" placeholder="0-10" value="8.5"`,
		`value="` + tagID.String() + `" checked>`,
		`href="/restaurants/` + restaurantID.String() + `/edit?return_visit_id=` + visitID.String() + `"`,
		`name="keep_photo_id" value="` + photoID.String() + `"`,
		`data-photo-preview data-photo-src="/photos/` + photoID.String() + `"`,
		note,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered visit edit missing %q:\n%s", fragment, rendered)
		}
	}
	addIndex := strings.Index(rendered, `data-photo-add`)
	photoIndex := strings.Index(rendered, `name="keep_photo_id" value="`+photoID.String()+`"`)
	if addIndex < 0 || photoIndex < 0 || addIndex > photoIndex {
		t.Fatalf("rendered visit edit should place add-photo tile before existing photos:\n%s", rendered)
	}
}
