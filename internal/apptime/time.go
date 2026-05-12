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

func loadEasternLocation() *time.Location {
	location, err := time.LoadLocation("America/New_York")
	if err == nil {
		return location
	}
	return time.FixedZone("EST", -5*60*60)
}
