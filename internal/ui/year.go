package ui

import (
	"fmt"
	"time"
)

// MonthBar is one month's column in the year in review chart.
type MonthBar struct {
	Label  string
	Visits int
	// Height is the column's height as a CSS percentage of the busiest month.
	Height string
}

// monthBars lays out a year's visits per month, January first. The busiest
// month fills the chart and a month with visits is never shorter than a sliver.
func monthBars(counts [12]int) []MonthBar {
	var busiest int
	for _, count := range counts {
		busiest = max(busiest, count)
	}
	bars := make([]MonthBar, len(counts))
	for i, count := range counts {
		height := 0
		if count > 0 {
			height = max(count*100/busiest, 6)
		}
		bars[i] = MonthBar{
			Label:  time.Month(i + 1).String()[:3],
			Visits: count,
			Height: fmt.Sprintf("%d%%", height),
		}
	}
	return bars
}
