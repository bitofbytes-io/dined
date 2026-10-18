package handler

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/ui"
)

// Query parameters of the /dines filters. People, tags, cuisines, and cities
// are named rather than identified so a filtered link stays readable.
const (
	cuisineParam = "cuisine"
	pickerParam  = "picker"
	raterParam   = "rater"
	tagParam     = "tag"
	cityParam    = "city"
	yearParam    = "year"
)

// dinesFilterParams lists the filter parameters in Filter dialog order:
// Cuisine and Picked by come first because they are the common filters.
var dinesFilterParams = []string{cuisineParam, pickerParam, raterParam, tagParam, cityParam, yearParam}

// dinesFilterValues holds the set /dines filter values, trimmed, by parameter.
type dinesFilterValues map[string]string

func readDinesFilterParams(query url.Values) dinesFilterValues {
	values := dinesFilterValues{}
	for _, name := range dinesFilterParams {
		if value := strings.TrimSpace(query.Get(name)); value != "" {
			values[name] = value
		}
	}
	return values
}

func (v dinesFilterValues) get(name string) string {
	return v[name]
}

// active counts the filters that are set.
func (v dinesFilterValues) active() int {
	return len(v)
}

// url links to the given page of /dines with these filters.
func (v dinesFilterValues) url(page int) string {
	query := url.Values{}
	for name, value := range v {
		query.Set(name, value)
	}
	if page > 1 {
		query.Set("page", strconv.Itoa(page))
	}
	if len(query) == 0 {
		return "/dines"
	}
	return "/dines?" + query.Encode()
}

// dinesYear parses the optional year filter.
func dinesYear(value string) (int, bool) {
	if value == "" {
		return 0, true
	}
	year, err := strconv.Atoi(value)
	if err != nil || year < 1 || year > 9999 {
		return 0, false
	}
	return year, true
}

// dinesFilterChoices are what the /dines filters choose from.
type dinesFilterChoices struct {
	people  []model.Person
	tags    []model.Tag
	options model.VisitFilterOptions
}

func (h *Handler) dinesFilterChoices(r *http.Request) (dinesFilterChoices, error) {
	people, err := h.store.People(r.Context())
	if err != nil {
		return dinesFilterChoices{}, err
	}
	tags, err := h.store.Tags(r.Context())
	if err != nil {
		return dinesFilterChoices{}, err
	}
	options, err := h.store.VisitFilterOptions(r.Context())
	if err != nil {
		return dinesFilterChoices{}, err
	}
	return dinesFilterChoices{people: people, tags: tags, options: options}, nil
}

// filter turns the filter values into a VisitFilter. It reports false when the
// picker, rater, or tag names no one, so that no visit can match.
func (c dinesFilterChoices) filter(values dinesFilterValues, year int) (model.VisitFilter, bool) {
	filter := model.VisitFilter{Cuisine: values.get(cuisineParam), City: values.get(cityParam), Year: year}
	matchable := true
	if picker := values.get(pickerParam); strings.EqualFold(picker, ui.EverybodyPickerValue) {
		filter.EverybodyPicked = true
	} else if picker != "" {
		if person, ok := c.person(picker); ok {
			filter.PickerID = &person.ID
		} else {
			matchable = false
		}
	}
	if rater := values.get(raterParam); rater != "" {
		if person, ok := c.person(rater); ok {
			filter.RaterID = &person.ID
		} else {
			matchable = false
		}
	}
	if name := values.get(tagParam); name != "" {
		if tag, ok := c.tag(name); ok {
			filter.TagID = &tag.ID
		} else {
			matchable = false
		}
	}
	return filter, matchable
}

func (c dinesFilterChoices) person(name string) (model.Person, bool) {
	for _, person := range c.people {
		if strings.EqualFold(person.Name, name) {
			return person, true
		}
	}
	return model.Person{}, false
}

func (c dinesFilterChoices) tag(name string) (model.Tag, bool) {
	for _, tag := range c.tags {
		if strings.EqualFold(tag.Name, name) {
			return tag, true
		}
	}
	return model.Tag{}, false
}

// view builds the /dines count line, Filter dialog, and pager links.
func (c dinesFilterChoices) view(values dinesFilterValues, matched, total, page, pages int) *ui.DinesFilter {
	people := make([]ui.DinesFilterOption, len(c.people))
	for i, person := range c.people {
		people[i] = ui.DinesFilterOption{Value: person.Name, Label: person.Name}
	}
	tags := make([]ui.DinesFilterOption, len(c.tags))
	for i, tag := range c.tags {
		tags[i] = ui.DinesFilterOption{Value: tag.Name, Label: tag.Name}
	}
	years := make([]string, len(c.options.Years))
	for i, year := range c.options.Years {
		years[i] = strconv.Itoa(year)
	}
	pickers := append([]ui.DinesFilterOption{{Value: ui.EverybodyPickerValue, Label: "Everybody"}}, people...)
	fields := []ui.DinesFilterField{
		{Name: cuisineParam, Label: "Cuisine", Options: textOptions(c.options.Cuisines)},
		{Name: pickerParam, Label: "Picked by", Options: pickers},
		{Name: raterParam, Label: "Rated by", Options: append([]ui.DinesFilterOption(nil), people...)},
		{Name: tagParam, Label: "Tag", Options: tags},
		{Name: cityParam, Label: "City", Options: textOptions(c.options.Cities)},
		{Name: yearParam, Label: "Year", Options: textOptions(years)},
	}

	view := &ui.DinesFilter{Fields: fields, Active: values.active(), Page: page, Pages: pages}
	var summary []string
	for i := range view.Fields {
		field := &view.Fields[i]
		value := values.get(field.Name)
		if value == "" {
			continue
		}
		label := value
		for j := range field.Options {
			if strings.EqualFold(field.Options[j].Value, value) {
				field.Options[j].Selected = true
				label = field.Options[j].Label
			}
		}
		switch field.Name {
		case pickerParam:
			label += "'s picks"
		case raterParam:
			label = "Rated by " + label
		}
		summary = append(summary, label)
	}
	view.Summary = strings.Join(summary, " · ")

	noun := "dines"
	if total == 1 {
		noun = "dine"
	}
	view.Count = fmt.Sprintf("%d %s", total, noun)
	if view.Active > 0 {
		view.Count = fmt.Sprintf("%d of %d %s", matched, total, noun)
	}
	if page > 1 {
		view.PrevURL = values.url(page - 1)
	}
	if page < pages {
		view.NextURL = values.url(page + 1)
	}
	return view
}

// textOptions offers each value as both the option value and its label.
func textOptions(values []string) []ui.DinesFilterOption {
	options := make([]ui.DinesFilterOption, len(values))
	for i, value := range values {
		options[i] = ui.DinesFilterOption{Value: value, Label: value}
	}
	return options
}
