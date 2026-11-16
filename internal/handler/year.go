package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/ui"
	"github.com/go-chi/chi/v5"
)

// minReviewYear is the earliest year the year in review page serves.
const minReviewYear = 1970

// Year is the year in review for one Eastern calendar year. A year with no
// dines still renders, so the trophy case can always link to it.
func (h *Handler) Year(w http.ResponseWriter, r *http.Request) {
	year, ok := reviewYear(chi.URLParam(r, "year"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	review, err := h.store.YearInReview(r.Context(), year)
	if err != nil {
		h.error(w, "year in review", err)
		return
	}
	data := ui.PageData{
		Title:      strconv.Itoa(year) + " in Review",
		YearReview: &review,
	}
	if year > minReviewYear {
		data.YearPrev = year - 1
	}
	if year < currentYear(time.Now()) {
		data.YearNext = year + 1
	}
	h.render(w, "year", r, data)
}

// reviewYear parses the four digit year in /year/{yyyy}.
func reviewYear(value string) (int, bool) {
	if len(value) != 4 {
		return 0, false
	}
	year, err := strconv.Atoi(value)
	if err != nil || year < minReviewYear {
		return 0, false
	}
	return year, true
}

// currentYear is the Eastern calendar year of now.
func currentYear(now time.Time) int {
	return now.In(apptime.EasternLocation()).Year()
}

// trophyReviewYears are the years the trophy case links to: this one and the
// one before it.
func trophyReviewYears(now time.Time) []int {
	year := currentYear(now)
	return []int{year, year - 1}
}
