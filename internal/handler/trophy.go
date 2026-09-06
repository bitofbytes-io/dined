package handler

import (
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/bitofbytes-io/dined/internal/ui"
)

type trophyMapCacheEntry struct {
	key       string
	image     places.StaticMapImage
	expiresAt time.Time
}

const (
	trophyMapCacheTTL   = 5 * time.Minute
	trophyMapWidth      = 640
	trophyMapHeight     = 360
	trophyMapPaddingX   = 58.0
	trophyMapPaddingY   = 74.0
	trophyMapMaxZoom    = 13
	trophyMapMaxMarkers = 100
	trophyMapLabelMax   = 16
)

func (h *Handler) Trophy(w http.ResponseWriter, r *http.Request) {
	stats, err := h.store.Stats(r.Context())
	if err != nil {
		h.error(w, "stats", err)
		return
	}
	pickerTurn, err := h.store.PickerTurn(r.Context())
	if err != nil {
		h.error(w, "picker turn", err)
		return
	}
	mapPoints, err := h.store.VisitedRestaurantMapPoints(r.Context())
	if err != nil {
		h.error(w, "visited restaurant map points", err)
		return
	}
	data := ui.PageData{
		Title:           "Trophy Case",
		Stats:           stats,
		PickerTurn:      pickerTurn,
		TrophyMapPoints: mapPoints,
		TrophyMapLabels: trophyMapLabels(mapPoints),
	}
	switch {
	case len(mapPoints) == 0:
		data.TrophyMapFallback = "No mapped dines yet"
	case !h.placesConfigured():
		data.TrophyMapFallback = "Map unavailable until Google Places is configured"
	default:
		data.TrophyMapReady = true
	}
	h.render(w, "trophy", r, data)
}

func (h *Handler) TrophyMap(w http.ResponseWriter, r *http.Request) {
	mapPoints, err := h.store.VisitedRestaurantMapPoints(r.Context())
	if err != nil {
		h.error(w, "visited restaurant map points", err)
		return
	}
	if len(mapPoints) == 0 {
		http.NotFound(w, r)
		return
	}
	if !h.placesConfigured() {
		http.Error(w, "Map service is not configured", http.StatusServiceUnavailable)
		return
	}
	cacheKey := trophyMapCacheKey(mapPoints)
	if image, ok := h.cachedTrophyMap(cacheKey, time.Now()); ok {
		writeStaticMapImage(w, image)
		return
	}
	image, err := h.places.StaticMap(r.Context(), staticMapRequest(mapPoints))
	if err != nil {
		slog.Warn("static map failed", "error", err)
		http.Error(w, "Map unavailable", http.StatusBadGateway)
		return
	}
	if image == nil || len(image.Data) == 0 {
		slog.Warn("static map returned empty image")
		http.Error(w, "Map unavailable", http.StatusBadGateway)
		return
	}
	h.cacheTrophyMap(cacheKey, *image, time.Now())
	writeStaticMapImage(w, *image)
}

func (h *Handler) cachedTrophyMap(key string, now time.Time) (places.StaticMapImage, bool) {
	h.mapCacheMu.Lock()
	defer h.mapCacheMu.Unlock()
	if h.mapCache.key != key || now.After(h.mapCache.expiresAt) {
		return places.StaticMapImage{}, false
	}
	return h.mapCache.image, true
}

func (h *Handler) cacheTrophyMap(key string, image places.StaticMapImage, now time.Time) {
	h.mapCacheMu.Lock()
	defer h.mapCacheMu.Unlock()
	h.mapCache = trophyMapCacheEntry{
		key:       key,
		image:     image,
		expiresAt: now.Add(trophyMapCacheTTL),
	}
}

func writeStaticMapImage(w http.ResponseWriter, image places.StaticMapImage) {
	contentType := strings.TrimSpace(image.ContentType)
	if contentType == "" {
		contentType = "image/png"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(image.Data)
}

func trophyMapCacheKey(points []model.RestaurantMapPoint) string {
	var key strings.Builder
	for _, point := range points {
		fmt.Fprintf(&key, "%s|%.6f|%.6f|%d|%d\n", point.RestaurantID, point.Latitude, point.Longitude, point.VisitCount, point.LatestVisitedAt.UnixNano())
	}
	return key.String()
}

func staticMapRequest(points []model.RestaurantMapPoint) places.StaticMapRequest {
	visiblePoints := trophyMapVisiblePoints(points)
	markers := make([]places.StaticMapMarker, 0, len(visiblePoints))
	for _, point := range visiblePoints {
		markers = append(markers, places.StaticMapMarker{
			Latitude:  point.Latitude,
			Longitude: point.Longitude,
		})
	}
	request := places.StaticMapRequest{Markers: markers}
	if viewport, ok := trophyMapViewport(visiblePoints); ok {
		request.Viewport = &viewport
	}
	return request
}

func trophyMapVisiblePoints(points []model.RestaurantMapPoint) []model.RestaurantMapPoint {
	if len(points) > trophyMapMaxMarkers {
		return points[:trophyMapMaxMarkers]
	}
	return points
}

func trophyMapLabels(points []model.RestaurantMapPoint) []ui.TrophyMapLabel {
	visiblePoints := trophyMapVisiblePoints(points)
	viewport, ok := trophyMapViewport(visiblePoints)
	if !ok {
		return nil
	}
	labels := make([]ui.TrophyMapLabel, 0, len(visiblePoints))
	for _, point := range visiblePoints {
		left, top := trophyMapPointPercent(point.Latitude, point.Longitude, viewport)
		labels = append(labels, ui.TrophyMapLabel{
			Name: truncateMapLabel(point.Name),
			Left: fmt.Sprintf("%.3f%%", left),
			Top:  fmt.Sprintf("%.3f%%", top),
		})
	}
	return labels
}

func trophyMapViewport(points []model.RestaurantMapPoint) (places.StaticMapViewport, bool) {
	if len(points) == 0 {
		return places.StaticMapViewport{}, false
	}
	if len(points) == 1 {
		return places.StaticMapViewport{
			Latitude:  points[0].Latitude,
			Longitude: points[0].Longitude,
			Zoom:      trophyMapMaxZoom,
		}, true
	}

	minX, maxX := math.MaxFloat64, -math.MaxFloat64
	minY, maxY := math.MaxFloat64, -math.MaxFloat64
	for _, point := range points {
		x, y := mercatorUnit(point.Latitude, point.Longitude)
		minX = math.Min(minX, x)
		maxX = math.Max(maxX, x)
		minY = math.Min(minY, y)
		maxY = math.Max(maxY, y)
	}

	zoom := 0
	for candidate := trophyMapMaxZoom; candidate >= 0; candidate-- {
		worldSize := mercatorWorldSize(candidate)
		if (maxX-minX)*worldSize <= trophyMapWidth-2*trophyMapPaddingX &&
			(maxY-minY)*worldSize <= trophyMapHeight-2*trophyMapPaddingY {
			zoom = candidate
			break
		}
	}

	latitude, longitude := mercatorLatLng((minX+maxX)/2, (minY+maxY)/2)
	return places.StaticMapViewport{Latitude: latitude, Longitude: longitude, Zoom: zoom}, true
}

func trophyMapPointPercent(latitude, longitude float64, viewport places.StaticMapViewport) (float64, float64) {
	centerX, centerY := mercatorPixel(viewport.Latitude, viewport.Longitude, viewport.Zoom)
	pointX, pointY := mercatorPixel(latitude, longitude, viewport.Zoom)
	left := ((pointX - centerX) + trophyMapWidth/2) / trophyMapWidth * 100
	top := ((pointY - centerY) + trophyMapHeight/2) / trophyMapHeight * 100
	return math.Max(0, math.Min(100, left)), math.Max(0, math.Min(100, top))
}

func mercatorPixel(latitude, longitude float64, zoom int) (float64, float64) {
	x, y := mercatorUnit(latitude, longitude)
	worldSize := mercatorWorldSize(zoom)
	return x * worldSize, y * worldSize
}

func mercatorUnit(latitude, longitude float64) (float64, float64) {
	sinLat := math.Sin(latitude * math.Pi / 180)
	sinLat = math.Max(-0.9999, math.Min(0.9999, sinLat))
	x := (longitude + 180) / 360
	y := 0.5 - math.Log((1+sinLat)/(1-sinLat))/(4*math.Pi)
	return x, y
}

func mercatorLatLng(x, y float64) (float64, float64) {
	longitude := x*360 - 180
	latitude := math.Atan(math.Sinh(math.Pi*(1-2*y))) * 180 / math.Pi
	return latitude, longitude
}

func mercatorWorldSize(zoom int) float64 {
	return 256 * math.Pow(2, float64(zoom))
}

func truncateMapLabel(value string) string {
	label := strings.Join(strings.Fields(value), " ")
	runes := []rune(label)
	if len(runes) <= trophyMapLabelMax {
		return label
	}
	return string(runes[:trophyMapLabelMax-3]) + "..."
}
