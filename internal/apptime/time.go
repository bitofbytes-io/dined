package apptime

import "time"

const datetimeLocalLayout = "2006-01-02T15:04"

var easternLocation = loadEasternLocation()

func EasternLocation() *time.Location {
	return easternLocation
}

func FormatDatetimeLocal(t time.Time) string {
	return t.In(EasternLocation()).Format(datetimeLocalLayout)
}

func ParseDatetimeLocal(value string) (time.Time, error) {
	return time.ParseInLocation(datetimeLocalLayout, value, EasternLocation())
}

// YearBounds returns the start of the Eastern calendar year and the start of
// the next one.
func YearBounds(year int) (time.Time, time.Time) {
	start := time.Date(year, time.January, 1, 0, 0, 0, 0, EasternLocation())
	return start, start.AddDate(1, 0, 0)
}

func loadEasternLocation() *time.Location {
	location, err := time.LoadLocation("America/New_York")
	if err == nil {
		return location
	}
	return time.FixedZone("EST", -5*60*60)
}
