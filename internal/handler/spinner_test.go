package handler

import (
	"slices"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/ui"
	"github.com/google/uuid"
)

func spinTestRestaurant(name, category, city string) model.Restaurant {
	restaurant := model.Restaurant{ID: uuid.New(), Name: name}
	if category != "" {
		restaurant.Category = &category
	}
	if city != "" {
		restaurant.City = &city
	}
	return restaurant
}

func spinChoiceNames(choices []ui.SpinChoice) []string {
	names := make([]string, len(choices))
	for i, choice := range choices {
		names[i] = choice.Restaurant.Name
	}
	return names
}

func TestSpinChoicesMergesOneChoicePerRestaurant(t *testing.T) {
	tacos := spinTestRestaurant("Taco Spot", "Mexican", "Apex")
	pho := spinTestRestaurant("Pho Place", "Vietnamese", "Cary")
	diner := spinTestRestaurant("Hank's", "American", "Raleigh")
	items := []model.WantToTryItem{{ID: uuid.New(), Restaurant: tacos}, {ID: uuid.New(), Restaurant: pho}}
	// Pho Place went back on the list after a dine; it is one choice, not two.
	favorites := []model.RestedFavorite{{Restaurant: diner, AverageRating: 8}, {Restaurant: pho, AverageRating: 9}}

	choices := spinChoices(items, favorites)
	if got, want := spinChoiceNames(choices), []string{"Taco Spot", "Pho Place", "Hank's"}; !slices.Equal(got, want) {
		t.Fatalf("choices = %v, want %v", got, want)
	}
	if choices[1].WantToTry == nil || choices[1].Favorite == nil || choices[1].Favorite.AverageRating != 9 {
		t.Fatalf("Pho Place = %#v, want both its list item and favorite", choices[1])
	}
	if choices[0].Favorite != nil || choices[2].WantToTry != nil {
		t.Fatalf("choices = %#v, want only Pho Place to be both", choices)
	}
}

func TestSpinPageFiltersByCuisineOrCity(t *testing.T) {
	choices := spinChoices([]model.WantToTryItem{
		{Restaurant: spinTestRestaurant("Taco Spot", "Mexican", "Apex")},
		{Restaurant: spinTestRestaurant("Taqueria", " mexican ", "Raleigh")},
		{Restaurant: spinTestRestaurant("No Details", "", "")},
	}, []model.RestedFavorite{{Restaurant: spinTestRestaurant("Hank's", "American", "Raleigh")}})

	spin, matching := spinPage(choices, "")
	if spin.Total != 4 || len(matching) != 4 {
		t.Fatalf("total %d, matching %d; want 4 and 4 without a filter", spin.Total, len(matching))
	}
	var cuisines, cities []string
	for _, option := range spin.Cuisines {
		cuisines = append(cuisines, option.Value+"="+option.Label)
	}
	for _, option := range spin.Cities {
		cities = append(cities, option.Value)
	}
	if want := []string{"cuisine:American=American", "cuisine:Mexican=Mexican"}; !slices.Equal(cuisines, want) {
		t.Fatalf("cuisines = %v, want %v", cuisines, want)
	}
	if want := []string{"city:Apex", "city:Raleigh"}; !slices.Equal(cities, want) {
		t.Fatalf("cities = %v, want %v", cities, want)
	}

	spin, matching = spinPage(choices, "cuisine:mexican")
	if got, want := spinChoiceNames(matching), []string{"Taco Spot", "Taqueria"}; !slices.Equal(got, want) {
		t.Fatalf("Mexican = %v, want %v", got, want)
	}
	if !spin.Cuisines[1].Selected || spin.Cuisines[0].Selected {
		t.Fatalf("cuisines = %#v, want Mexican selected", spin.Cuisines)
	}

	_, matching = spinPage(choices, "city:Raleigh")
	if got, want := spinChoiceNames(matching), []string{"Taqueria", "Hank's"}; !slices.Equal(got, want) {
		t.Fatalf("Raleigh = %v, want %v", got, want)
	}

	// A filter for nothing on the list now means Anywhere.
	spin, matching = spinPage(choices, "city:Durham")
	if len(matching) != 4 {
		t.Fatalf("stale filter left %v, want every choice", spinChoiceNames(matching))
	}
	for _, option := range append(spin.Cuisines, spin.Cities...) {
		if option.Selected {
			t.Fatalf("stale filter selected %q", option.Value)
		}
	}
}

func TestPickSpinChoiceSkipsTheLastPick(t *testing.T) {
	choices := spinChoices([]model.WantToTryItem{
		{Restaurant: spinTestRestaurant("First", "", "")},
		{Restaurant: spinTestRestaurant("Second", "", "")},
	}, nil)
	first := func(int) int { return 0 }
	if got := pickSpinChoice(choices, choices[0].Restaurant.ID, first); got.Restaurant.Name != "Second" {
		t.Fatalf("picked %s again, want Second", got.Restaurant.Name)
	}
	if got := pickSpinChoice(choices, uuid.Nil, first); got.Restaurant.Name != "First" {
		t.Fatalf("picked %s, want First", got.Restaurant.Name)
	}
	// With only one choice, the spin lands on it again.
	if got := pickSpinChoice(choices[:1], choices[0].Restaurant.ID, first); got.Restaurant.Name != "First" {
		t.Fatalf("picked %s, want First", got.Restaurant.Name)
	}
}

func TestSpinReelLeavesOutThePick(t *testing.T) {
	var items []model.WantToTryItem
	for _, name := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J"} {
		items = append(items, model.WantToTryItem{Restaurant: spinTestRestaurant(name, "", "")})
	}
	choices := spinChoices(items, nil)
	reel := spinReel(choices, choices[0])
	if len(reel) != spinReelSize {
		t.Fatalf("reel = %v, want %d names", reel, spinReelSize)
	}
	for _, name := range reel {
		if name == "A" {
			t.Fatalf("reel = %v, want the pick left out", reel)
		}
	}
	if reel := spinReel(choices[:1], choices[0]); len(reel) != 0 {
		t.Fatalf("reel = %v, want none for a single choice", reel)
	}
}

func TestWhoseTurnHasFewestRecentPicks(t *testing.T) {
	at := func(day int) *time.Time {
		when := time.Date(2026, 9, day, 18, 0, 0, 0, time.UTC)
		return &when
	}
	person := func(name string) model.Person { return model.Person{ID: uuid.New(), Name: name} }
	people := []model.Person{person("Daniel"), person("Jen"), person("Caleb"), person("Aiden")}
	tests := []struct {
		name  string
		picks [4]int
		last  [4]*time.Time
		want  string
	}{
		{"fewest picks", [4]int{3, 1, 2, 4}, [4]*time.Time{at(20), at(10), at(5), at(1)}, "Jen"},
		{"tie goes to the longest wait", [4]int{1, 1, 1, 2}, [4]*time.Time{at(20), at(10), at(15), at(1)}, "Jen"},
		{"never picked waits longest", [4]int{}, [4]*time.Time{at(1), nil, nil, at(2)}, "Jen"},
		{"then people order", [4]int{}, [4]*time.Time{}, "Daniel"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			counts := make([]model.PickCount, len(people))
			for i, p := range people {
				counts[i] = model.PickCount{Person: p, Picks: tt.picks[i], LastPickedAt: tt.last[i]}
			}
			got := whoseTurn(counts)
			if got == nil || got.Name != tt.want {
				t.Fatalf("whose turn = %v, want %s", got, tt.want)
			}
		})
	}
	if got := whoseTurn(nil); got != nil {
		t.Fatalf("whose turn with no people = %v, want nil", got)
	}
}
