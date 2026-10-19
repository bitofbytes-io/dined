package handler

import (
	"log/slog"
	"math/rand/v2"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/ui"
	"github.com/google/uuid"
)

const (
	// spinMinAverage is the lowest average, over visits at least two people
	// rated, that puts a place we have dined at in the spinner.
	spinMinAverage = 7.0
	// spinRestDays is how long after a dine a favorite stays out of the spinner.
	spinRestDays = 30
	// spinTurnDays is how far back whose turn it is counts picks.
	spinTurnDays = 90
	// spinReelSize caps the other names the result card flashes through.
	spinReelSize = 8

	cuisineSpinFilter = "cuisine:"
	citySpinFilter    = "city:"
)

// Spinner is "Where should we eat?": a random pick from the Want to Try list
// and well-rated places we have not been to lately, optionally narrowed to one
// cuisine or city, with whose turn it would be to pick instead.
func (h *Handler) Spinner(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now()
	items, err := h.store.WantToTry(ctx)
	if err != nil {
		h.error(w, "spinner want to try", err)
		return
	}
	favorites, err := h.store.RestedFavorites(ctx, spinMinAverage, now.AddDate(0, 0, -spinRestDays))
	if err != nil {
		h.error(w, "spinner favorites", err)
		return
	}
	choices := spinChoices(items, favorites)
	query := r.URL.Query()
	spin, matching := spinPage(choices, query.Get("from"))
	if query.Get("spin") != "" && len(matching) > 0 {
		last, _ := uuid.Parse(query.Get("last"))
		pick := pickSpinChoice(matching, last, rand.IntN)
		spin.Pick = &pick
		spin.Reel = spinReel(matching, pick)
		counts, err := h.store.PickCounts(ctx, now.AddDate(0, 0, -spinTurnDays))
		if err != nil {
			// The pick still stands without whose turn it is.
			slog.Warn("spinner pick counts", "error", err)
		}
		spin.Turn = whoseTurn(counts)
	}
	h.render(w, "spinner", r, ui.PageData{Title: "Where should we eat?", Spin: &spin})
}

// spinChoices merges the open Want to Try items and the rested favorites into
// one choice per restaurant: the list's items first, newest first, then the
// favorites by name.
func spinChoices(items []model.WantToTryItem, favorites []model.RestedFavorite) []ui.SpinChoice {
	choices := make([]ui.SpinChoice, 0, len(items)+len(favorites))
	byRestaurant := make(map[uuid.UUID]int, len(items)+len(favorites))
	for i := range items {
		byRestaurant[items[i].Restaurant.ID] = len(choices)
		choices = append(choices, ui.SpinChoice{Restaurant: items[i].Restaurant, WantToTry: &items[i]})
	}
	for i := range favorites {
		if index, ok := byRestaurant[favorites[i].Restaurant.ID]; ok {
			choices[index].Favorite = &favorites[i]
			continue
		}
		byRestaurant[favorites[i].Restaurant.ID] = len(choices)
		choices = append(choices, ui.SpinChoice{Restaurant: favorites[i].Restaurant, Favorite: &favorites[i]})
	}
	return choices
}

// spinPage builds the page's filter dropdown from the choices and returns the
// choices the filter leaves. A filter that names no choice's cuisine or city,
// say one saved from an older list, means Anywhere.
func spinPage(choices []ui.SpinChoice, filter string) (ui.Spin, []ui.SpinChoice) {
	cuisines, cities := map[string]string{}, map[string]string{}
	for _, choice := range choices {
		keepSpinValue(cuisines, choice.Restaurant.Category)
		keepSpinValue(cities, choice.Restaurant.City)
	}
	spin := ui.Spin{
		Total:      len(choices),
		Cuisines:   spinOptions(cuisines, cuisineSpinFilter),
		Cities:     spinOptions(cities, citySpinFilter),
		MinAverage: spinMinAverage,
		RestDays:   spinRestDays,
		TurnDays:   spinTurnDays,
	}
	filter = strings.TrimSpace(filter)
	for _, options := range [][]ui.SpinOption{spin.Cuisines, spin.Cities} {
		for i := range options {
			if strings.EqualFold(options[i].Value, filter) {
				options[i].Selected = true
				return spin, filterSpinChoices(choices, options[i].Value)
			}
		}
	}
	return spin, choices
}

// keepSpinValue records a trimmed cuisine or city under its lowercase key,
// keeping one spelling per value as the /dines filters do.
func keepSpinValue(values map[string]string, value *string) {
	trimmed := strings.TrimSpace(valueOrEmpty(value))
	if trimmed == "" {
		return
	}
	key := strings.ToLower(trimmed)
	if existing, ok := values[key]; !ok || trimmed < existing {
		values[key] = trimmed
	}
}

func spinOptions(values map[string]string, prefix string) []ui.SpinOption {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	options := make([]ui.SpinOption, len(keys))
	for i, key := range keys {
		options[i] = ui.SpinOption{Value: prefix + values[key], Label: values[key]}
	}
	return options
}

// filterSpinChoices keeps the choices in one cuisine or city, as a filter
// value such as "cuisine:Mexican" or "city:Apex" names it.
func filterSpinChoices(choices []ui.SpinChoice, filter string) []ui.SpinChoice {
	field := func(restaurant model.Restaurant) *string { return restaurant.Category }
	value, ok := strings.CutPrefix(filter, cuisineSpinFilter)
	if !ok {
		value, ok = strings.CutPrefix(filter, citySpinFilter)
		field = func(restaurant model.Restaurant) *string { return restaurant.City }
	}
	if !ok {
		return choices
	}
	var matching []ui.SpinChoice
	for _, choice := range choices {
		if strings.EqualFold(strings.TrimSpace(valueOrEmpty(field(choice.Restaurant))), value) {
			matching = append(matching, choice)
		}
	}
	return matching
}

// pickSpinChoice picks one choice with intn, skipping the restaurant the last
// spin landed on when there is anything else to land on.
func pickSpinChoice(choices []ui.SpinChoice, last uuid.UUID, intn func(int) int) ui.SpinChoice {
	candidates := choices
	if len(choices) > 1 {
		candidates = make([]ui.SpinChoice, 0, len(choices))
		for _, choice := range choices {
			if choice.Restaurant.ID != last {
				candidates = append(candidates, choice)
			}
		}
		if len(candidates) == 0 {
			candidates = choices
		}
	}
	return candidates[intn(len(candidates))]
}

// spinReel lists, in random order, up to spinReelSize names of the other
// choices for the result card to flash through.
func spinReel(choices []ui.SpinChoice, pick ui.SpinChoice) []string {
	var names []string
	for _, choice := range choices {
		if choice.Restaurant.ID != pick.Restaurant.ID {
			names = append(names, choice.Restaurant.Name)
		}
	}
	rand.Shuffle(len(names), func(i, j int) { names[i], names[j] = names[j], names[i] })
	if len(names) > spinReelSize {
		names = names[:spinReelSize]
	}
	return names
}

// whoseTurn is the person with the fewest recent picks. Ties go to whoever
// picked longest ago, someone who never picked first, and then to people order.
func whoseTurn(counts []model.PickCount) *model.Person {
	var best *model.PickCount
	for i := range counts {
		count := &counts[i]
		if best == nil || count.Picks < best.Picks ||
			(count.Picks == best.Picks && pickedEarlier(count.LastPickedAt, best.LastPickedAt)) {
			best = count
		}
	}
	if best == nil {
		return nil
	}
	return &best.Person
}

// pickedEarlier reports whether a last pick at a comes strictly before one at
// b, where nil means never.
func pickedEarlier(a, b *time.Time) bool {
	if a == nil {
		return b != nil
	}
	return b != nil && a.Before(*b)
}
