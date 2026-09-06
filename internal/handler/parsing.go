package handler

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func optionalFloat(value, label string) (*float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil, fmt.Errorf("%s must be a number", label)
	}
	return &parsed, nil
}

func optionalCoordinate(value, label string, min float64, max float64) (*float64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := parseCoordinate(value, label, min, max)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseCoordinate(value, label string, min float64, max float64) (float64, error) {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", label)
	}
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0, fmt.Errorf("%s must be a finite number", label)
	}
	if parsed < min || parsed > max {
		return 0, fmt.Errorf("%s must be between %.0f and %.0f", label, min, max)
	}
	return parsed, nil
}

func optionalInt(value, label string) (*int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return nil, fmt.Errorf("%s must be a number", label)
	}
	return &parsed, nil
}
