package handler

import "testing"

func TestNearbyTextQuery(t *testing.T) {
	tests := []struct {
		name  string
		query string
		near  string
		want  string
	}{
		{name: "restaurant near place", query: "tacos", near: "Brooklyn", want: "tacos near Brooklyn"},
		{name: "place only", near: "10001", want: "restaurants near 10001"},
		{name: "query only", query: "ramen", want: "ramen"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nearbyTextQuery(tt.query, tt.near); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGoogleRefreshNotice(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{status: "updated", want: "Google info refreshed"},
		{status: "failed", want: "Google info refresh failed"},
		{status: "missing-place-id", want: "No Google Place ID saved for this restaurant"},
		{status: "unconfigured", want: "Google Places is not configured"},
		{status: "ignored", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := googleRefreshNotice(tt.status); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
