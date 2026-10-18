package model

import "github.com/google/uuid"

// VisitFilter narrows a visit list. The zero value matches every visit, and
// the filters that are set combine with AND.
type VisitFilter struct {
	// Cuisine matches the restaurant category, trimmed and case-insensitive.
	Cuisine string
	// EverybodyPicked matches visits that Everybody picked. It takes
	// precedence over PickerID.
	EverybodyPicked bool
	PickerID        *uuid.UUID
	// RaterID matches visits the person rated at all.
	RaterID *uuid.UUID
	TagID   *uuid.UUID
	// City matches the restaurant city, trimmed and case-insensitive.
	City string
	// Year matches the calendar year of the visit in Eastern time.
	Year int
}

// VisitFilterOptions are the cuisines, cities, and years that appear on at
// least one visit, for building filter choices.
type VisitFilterOptions struct {
	// Cuisines and Cities are trimmed, one spelling per case-insensitive
	// value, sorted case-insensitively.
	Cuisines []string
	Cities   []string
	// Years are newest first.
	Years []int
}
