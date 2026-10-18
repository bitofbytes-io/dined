package repository

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

// These checks run against both stores so the memory preview filters /dines
// the way Postgres does. Each expects a store with the four seeded people,
// the seeded tags, and no visits.

type filterTestVisit struct {
	name, category, city string
	at                   time.Time
	picker               *model.Person
	raters               []model.Person
	tags                 []model.Tag
}

func createFilterTestVisit(t *testing.T, store DinerStore, visit filterTestVisit) uuid.UUID {
	t.Helper()
	input := model.VisitInput{
		RestaurantName: visit.name,
		Address:        "1 " + visit.name + " Way", // Same name and address: the same restaurant.
		City:           visit.city,
		Category:       visit.category,
		VisitedAt:      visit.at,
		PriceLevel:     2,
		Ratings:        map[uuid.UUID]float64{},
	}
	if visit.picker != nil {
		input.PickerID = &visit.picker.ID
	}
	for _, rater := range visit.raters {
		input.Ratings[rater.ID] = 8
	}
	for _, tag := range visit.tags {
		input.TagIDs = append(input.TagIDs, tag.ID)
	}
	id, err := store.CreateVisit(context.Background(), input)
	if err != nil {
		t.Fatalf("create %s: %v", visit.name, err)
	}
	return *id
}

func storeTag(t *testing.T, store DinerStore, name string) model.Tag {
	t.Helper()
	tags, err := store.Tags(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, tag := range tags {
		if tag.Name == name {
			return tag
		}
	}
	t.Fatalf("no %q tag in %#v", name, tags)
	return model.Tag{}
}

func easternTime(year int, month time.Month, day, hour int) time.Time {
	return time.Date(year, month, day, hour, 0, 0, 0, apptime.EasternLocation())
}

func assertVisitFilters(t *testing.T, store DinerStore) {
	t.Helper()
	ctx := context.Background()
	daniel, jen, caleb, aiden := storePeople(t, store)
	kidApproved := storeTag(t, store, "Kid Approved")
	wouldReturn := storeTag(t, store, "Would Return")

	// Newest first: lateTaco, taco, noodles, everybodyTaco, pizza.
	taco := createFilterTestVisit(t, store, filterTestVisit{name: "Taco Spot", category: "Mexican", city: "Raleigh", at: easternTime(2026, 3, 10, 18),
		picker: &caleb, raters: []model.Person{daniel, caleb}, tags: []model.Tag{kidApproved}})
	everybodyTaco := createFilterTestVisit(t, store, filterTestVisit{name: "Taco Two", category: " mexican ", city: "raleigh", at: easternTime(2025, 6, 1, 18),
		raters: []model.Person{jen, aiden}, tags: []model.Tag{kidApproved, wouldReturn}})
	// 03:00 UTC on New Year's Day is still New Year's Eve in Eastern time.
	noodles := createFilterTestVisit(t, store, filterTestVisit{name: "Noodle Bar", category: "Thai", city: "Cary", at: time.Date(2026, 1, 1, 3, 0, 0, 0, time.UTC),
		picker: &caleb, raters: []model.Person{jen}})
	pizza := createFilterTestVisit(t, store, filterTestVisit{name: "Pizza Place", at: easternTime(2024, 7, 4, 18),
		picker: &jen, raters: []model.Person{daniel, jen}, tags: []model.Tag{wouldReturn}})
	lateTaco := createFilterTestVisit(t, store, filterTestVisit{name: "Taco Spot", category: "Mexican", city: "Raleigh", at: easternTime(2026, 8, 1, 18),
		picker: &daniel, raters: []model.Person{caleb}})
	// A restaurant whose only dine was deleted offers no filter choices.
	ghost := createFilterTestVisit(t, store, filterTestVisit{name: "Ghost Grill", category: "Ethiopian", city: "Durham", at: easternTime(2023, 5, 5, 18),
		picker: &aiden, raters: []model.Person{aiden}})
	if err := store.DeleteVisit(ctx, ghost); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		filter model.VisitFilter
		want   []uuid.UUID
	}{
		{"no filters", model.VisitFilter{}, []uuid.UUID{lateTaco, taco, noodles, everybodyTaco, pizza}},
		{"cuisine ignores case and spaces", model.VisitFilter{Cuisine: " MEXICAN"}, []uuid.UUID{lateTaco, taco, everybodyTaco}},
		{"unknown cuisine", model.VisitFilter{Cuisine: "Ethiopian"}, nil},
		{"Everybody picked", model.VisitFilter{EverybodyPicked: true}, []uuid.UUID{everybodyTaco}},
		{"Everybody wins over a picker", model.VisitFilter{EverybodyPicked: true, PickerID: &caleb.ID}, []uuid.UUID{everybodyTaco}},
		{"picker", model.VisitFilter{PickerID: &caleb.ID}, []uuid.UUID{taco, noodles}},
		{"rater", model.VisitFilter{RaterID: &caleb.ID}, []uuid.UUID{lateTaco, taco}},
		{"tag", model.VisitFilter{TagID: &kidApproved.ID}, []uuid.UUID{taco, everybodyTaco}},
		{"city ignores case and spaces", model.VisitFilter{City: "Raleigh "}, []uuid.UUID{lateTaco, taco, everybodyTaco}},
		{"year in Eastern time", model.VisitFilter{Year: 2025}, []uuid.UUID{noodles, everybodyTaco}},
		{"year with no dines", model.VisitFilter{Year: 2023}, nil},
		{"cuisine and picker", model.VisitFilter{Cuisine: "Mexican", PickerID: &caleb.ID}, []uuid.UUID{taco}},
		{"cuisine, tag and year", model.VisitFilter{Cuisine: "mexican", TagID: &kidApproved.ID, Year: 2025}, []uuid.UUID{everybodyTaco}},
		{"every filter", model.VisitFilter{Cuisine: "Mexican", PickerID: &caleb.ID, RaterID: &daniel.ID, TagID: &kidApproved.ID, City: "Raleigh", Year: 2026}, []uuid.UUID{taco}},
		{"filters that exclude each other", model.VisitFilter{PickerID: &jen.ID, TagID: &kidApproved.ID}, nil},
	}
	for _, test := range tests {
		visits, err := store.VisitsPage(ctx, test.filter, 0, 0)
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		var got []uuid.UUID
		for _, visit := range visits {
			got = append(got, visit.ID)
		}
		if !slices.Equal(got, test.want) {
			t.Fatalf("%s: VisitsPage = %v, want %v", test.name, got, test.want)
		}
		count, err := store.CountVisits(ctx, test.filter)
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if count != len(test.want) {
			t.Fatalf("%s: CountVisits = %d, want %d", test.name, count, len(test.want))
		}
	}

	page, err := store.VisitsPage(ctx, model.VisitFilter{Cuisine: "Mexican"}, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].ID != taco || page[1].ID != everybodyTaco {
		t.Fatalf("filtered page = %#v, want the second and third Mexican dines", page)
	}
	if len(page[1].Ratings) != 2 || len(page[1].Tags) != 2 {
		t.Fatalf("filtered visit relations = %d ratings/%d tags, want 2/2", len(page[1].Ratings), len(page[1].Tags))
	}

	options, err := store.VisitFilterOptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Mexican", "Thai"}; !slices.Equal(options.Cuisines, want) {
		t.Fatalf("cuisines = %q, want %q", options.Cuisines, want)
	}
	if want := []string{"Cary", "Raleigh"}; !slices.Equal(options.Cities, want) {
		t.Fatalf("cities = %q, want %q", options.Cities, want)
	}
	if want := []int{2026, 2025, 2024}; !slices.Equal(options.Years, want) {
		t.Fatalf("years = %v, want %v", options.Years, want)
	}
}

func assertNoVisitFilterOptionsWithoutVisits(t *testing.T, store DinerStore) {
	t.Helper()
	options, err := store.VisitFilterOptions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Cuisines) != 0 || len(options.Cities) != 0 || len(options.Years) != 0 {
		t.Fatalf("options = %#v, want none", options)
	}
	if count, err := store.CountVisits(context.Background(), model.VisitFilter{}); err != nil || count != 0 {
		t.Fatalf("count = %d, err = %v; want 0", count, err)
	}
}
