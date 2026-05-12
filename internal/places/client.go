package places

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	apiKey string
	http   *http.Client
}

type Place struct {
	ID          string   `json:"id"`
	DisplayName Name     `json:"displayName"`
	Address     string   `json:"formattedAddress"`
	Location    Location `json:"location"`
	Phone       string   `json:"nationalPhoneNumber"`
	Website     string   `json:"websiteUri"`
	Rating      float64  `json:"rating"`
	PriceLevel  string   `json:"priceLevel"`
	Types       []string `json:"types"`
}

type Name struct {
	Text string `json:"text"`
}

type Location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type searchResponse struct {
	Places []Place `json:"places"`
}

const searchFieldMask = "places.id,places.displayName,places.formattedAddress,places.location,places.nationalPhoneNumber,places.websiteUri,places.rating,places.priceLevel,places.types"
const detailsFieldMask = "id,displayName,formattedAddress,location,nationalPhoneNumber,websiteUri,rating,priceLevel,types"

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		http: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

func (c *Client) Configured() bool {
	return strings.TrimSpace(c.apiKey) != ""
}

func (c *Client) TextSearch(ctx context.Context, query string) ([]Place, error) {
	return c.postSearch(ctx, "https://places.googleapis.com/v1/places:searchText", textSearchBody(query))
}

func (c *Client) TextSearchNear(ctx context.Context, query string, lat, lng float64) ([]Place, error) {
	return c.postSearch(ctx, "https://places.googleapis.com/v1/places:searchText", textSearchNearBody(query, lat, lng))
}

func textSearchNearBody(query string, lat, lng float64) map[string]any {
	body := textSearchBody(query)
	body["locationBias"] = map[string]any{
		"circle": map[string]any{
			"center": map[string]float64{
				"latitude":  lat,
				"longitude": lng,
			},
			"radius": 8047.0,
		},
	}
	return body
}

func textSearchBody(query string) map[string]any {
	return map[string]any{
		"textQuery":        query,
		"includedType":     "restaurant",
		"maxResultCount":   10,
		"rankPreference":   "RELEVANCE",
		"languageCode":     "en",
		"regionCode":       "US",
		"strictTypeFilter": false,
	}
}

func (c *Client) Nearby(ctx context.Context, lat, lng float64) ([]Place, error) {
	body := map[string]any{
		"includedTypes":  []string{"restaurant"},
		"maxResultCount": 10,
		"rankPreference": "DISTANCE",
		"locationRestriction": map[string]any{
			"circle": map[string]any{
				"center": map[string]float64{
					"latitude":  lat,
					"longitude": lng,
				},
				"radius": 1609.0,
			},
		},
	}
	return c.postSearch(ctx, "https://places.googleapis.com/v1/places:searchNearby", body)
}

func (c *Client) Details(ctx context.Context, placeID string) (*Place, error) {
	endpoint := "https://places.googleapis.com/v1/places/" + url.PathEscape(placeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	c.addHeaders(req, detailsFieldMask)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return nil, fmt.Errorf("google places details status %d: %s", res.StatusCode, string(data))
	}
	var place Place
	if err := json.NewDecoder(res.Body).Decode(&place); err != nil {
		return nil, err
	}
	return &place, nil
}

func (c *Client) postSearch(ctx context.Context, endpoint string, body any) ([]Place, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	c.addHeaders(req, searchFieldMask)
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		return nil, fmt.Errorf("google places search status %d: %s", res.StatusCode, string(data))
	}
	var parsed searchResponse
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	return parsed.Places, nil
}

func (c *Client) addHeaders(req *http.Request, fieldMask string) {
	req.Header.Set("X-Goog-Api-Key", c.apiKey)
	req.Header.Set("X-Goog-FieldMask", fieldMask)
}

func PriceLevelNumber(priceLevel string) int {
	switch priceLevel {
	case "PRICE_LEVEL_INEXPENSIVE":
		return 1
	case "PRICE_LEVEL_MODERATE":
		return 2
	case "PRICE_LEVEL_EXPENSIVE":
		return 3
	case "PRICE_LEVEL_VERY_EXPENSIVE":
		return 4
	default:
		return 0
	}
}

func DistanceMiles(lat, lng float64, place Place) float64 {
	const earthRadiusMiles = 3958.8
	lat1 := degreesToRadians(lat)
	lat2 := degreesToRadians(place.Location.Latitude)
	dLat := degreesToRadians(place.Location.Latitude - lat)
	dLng := degreesToRadians(place.Location.Longitude - lng)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return earthRadiusMiles * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func degreesToRadians(value float64) float64 {
	return value * math.Pi / 180
}
