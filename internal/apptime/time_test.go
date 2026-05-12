package apptime

import (
	"testing"
	"time"
)

func TestFormatDatetimeLocalUsesEasternTime(t *testing.T) {
	utc := time.Date(2026, 5, 12, 7, 50, 0, 0, time.UTC)

	if got := FormatDatetimeLocal(utc); got != "2026-05-12T03:50" {
		t.Fatalf("FormatDatetimeLocal() = %q, want Eastern local time", got)
	}
}

func TestParseDatetimeLocalUsesEasternTime(t *testing.T) {
	got, err := ParseDatetimeLocal("2026-05-12T03:50")
	if err != nil {
		t.Fatal(err)
	}

	name, offset := got.Zone()
	if name != "EDT" || offset != -4*60*60 {
		t.Fatalf("parsed zone = %s %d, want EDT -14400", name, offset)
	}
	if got.UTC().Format(time.RFC3339) != "2026-05-12T07:50:00Z" {
		t.Fatalf("parsed UTC = %s, want 2026-05-12T07:50:00Z", got.UTC().Format(time.RFC3339))
	}
}
