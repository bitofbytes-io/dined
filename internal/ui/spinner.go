package ui

import (
	"encoding/json"

	"github.com/bitofbytes-io/dined/internal/model"
)

// Spin is the "Where should we eat?" page: the places to choose from, the
// optional cuisine or city filter, and where the last spin landed.
type Spin struct {
	// Total counts every place the spinner can land on, before the filter.
	Total int
	// MinAverage and RestDays are the rules for places we have dined at: a
	// two-rater average of at least MinAverage, and no dine in RestDays days.
	MinAverage float64
	RestDays   int
	// Cuisines and Cities are the filter dropdown's choices after "Anywhere".
	Cuisines []SpinOption
	Cities   []SpinOption
	// Pick is where the spin landed, and Turn whose turn it would be to pick
	// instead. TurnDays is how many days of picks Turn counted.
	Pick     *SpinChoice
	Turn     *model.Person
	TurnDays int
	// Reel is a few other names the result card flashes through before
	// settling on the pick.
	Reel []string
}

// SpinOption is one choice in the filter dropdown.
type SpinOption struct {
	Value    string
	Label    string
	Selected bool
}

// SpinChoice is a place the spinner can land on: an open Want to Try item, a
// favorite the family has not been back to lately, or both.
type SpinChoice struct {
	Restaurant model.Restaurant
	WantToTry  *model.WantToTryItem
	Favorite   *model.RestedFavorite
}

// ReelJSON is Reel as a JSON array for the result card's data attribute.
func (s Spin) ReelJSON() string {
	if len(s.Reel) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(s.Reel)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}
