package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

func TestRenderRestaurantIsReadOnlyForAuthenticatedUsers(t *testing.T) {
	restaurantID := uuid.New()
	visitID := uuid.New()
	placeID := "place-1"
	var out strings.Builder
	err := Render(&out, "restaurant", PageData{
		Authenticated:  true,
		ReadOnlyVisits: true,
		Restaurant: &model.Restaurant{
			ID:            restaurantID,
			Name:          "Tupelo Honey Southern Kitchen & Bar",
			GooglePlaceID: &placeID,
			IsChain:       true,
		},
		Visits: []model.Visit{{
			ID:         visitID,
			Restaurant: model.Restaurant{ID: restaurantID, Name: "Tupelo Honey Southern Kitchen & Bar"},
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
	for _, fragment := range []string{"Tupelo Honey Southern Kitchen &amp; Bar", "Chain", "Daniel 8"} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered restaurant missing %q:\n%s", fragment, rendered)
		}
	}
	for _, fragment := range []string{
		"Edit Details",
		"Log Another Dine",
		"Refresh Google Info",
		"Mark Chain",
		"Clear Chain Badge",
		`href="/restaurants/` + restaurantID.String() + `/edit"`,
		`action="/restaurants/` + restaurantID.String() + `/google-refresh"`,
		`action="/restaurants/` + restaurantID.String() + `/chain"`,
		`href="/visits/` + visitID.String() + `/edit"`,
		`action="/visits/` + visitID.String() + `/delete"`,
	} {
		if strings.Contains(rendered, fragment) {
			t.Fatalf("rendered read-only restaurant included %q:\n%s", fragment, rendered)
		}
	}
}

func TestRenderRestaurantEditPrefillsOverrideFields(t *testing.T) {
	restaurantID := uuid.New()
	address := "202 Corrected Way"
	city := "Garner"
	phone := "919-555-0199"
	website := "https://hanks.example"
	category := "Southern"
	rating := 4.9
	price := 2
	placeID := "place-1"
	returnVisitID := uuid.New().String()
	var out strings.Builder
	err := Render(&out, "restaurant-edit", PageData{
		ReturnVisitID: returnVisitID,
		Restaurant: &model.Restaurant{
			ID:               restaurantID,
			Name:             "Hank's Corrected Diner",
			Address:          &address,
			City:             &city,
			Phone:            &phone,
			Website:          &website,
			GooglePlaceID:    &placeID,
			GoogleRating:     &rating,
			GooglePriceLevel: &price,
			Category:         &category,
			IsChain:          true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	rendered := out.String()
	for _, fragment := range []string{
		`action="/restaurants/` + restaurantID.String() + `"`,
		`id="restaurant-google-refresh-form" method="post" action="/restaurants/` + restaurantID.String() + `/google-refresh"`,
		`name="return_visit_id" value="` + returnVisitID + `"`,
		`href="/visits/` + returnVisitID + `/edit">Return to Edit Dine</a>`,
		`name="restaurant_name" value="Hank&#39;s Corrected Diner"`,
		`name="category" placeholder="American, Southern, Sushi..." value="Southern"`,
		`name="address" placeholder="Optional" value="202 Corrected Way"`,
		`name="phone" placeholder="Optional" value="919-555-0199"`,
		`name="website" type="url" placeholder="https://example.com" value="https://hanks.example"`,
		`name="google_rating" type="number" min="0" max="5" step="0.1" inputmode="decimal" placeholder="0-5" value="4.9"`,
		`value="2" selected>$$</option>`,
		`form="restaurant-google-refresh-form">Refresh Google Info</button>`,
		`name="is_chain" value="true" checked>`,
	} {
		if !strings.Contains(rendered, fragment) {
			t.Fatalf("rendered restaurant edit missing %q:\n%s", fragment, rendered)
		}
	}
}
