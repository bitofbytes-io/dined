package ui

import (
	"embed"
	"fmt"
	"html/template"
	"io"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/google/uuid"
)

//go:embed templates/*.html
var templates embed.FS

// pageTemplates is parsed once; html/template is safe for concurrent use.
var pageTemplates = template.Must(template.New("dined").Funcs(funcs()).ParseFS(templates, "templates/*.html"))

// assetVersions maps each /static/ URL to its cache-busted form. It is set
// once at startup by LoadAssetVersions.
var assetVersions atomic.Pointer[map[string]string]

type PageData struct {
	Title                   string
	Authenticated           bool
	Error                   string
	Notice                  string
	Visits                  []model.Visit
	ReadOnlyVisits          bool
	PrevPage                int
	NextPage                int
	DinesURL                string
	Visit                   *model.Visit
	People                  []model.Person
	Tags                    []model.Tag
	Restaurants             []model.Restaurant
	SearchResults           []RestaurantResult
	Restaurant              *model.Restaurant
	Stats                   model.Stats
	PickerTurn              model.PickerTurn
	TrophyMapPoints         []model.RestaurantMapPoint
	TrophyMapLabels         []TrophyMapLabel
	TrophyMapReady          bool
	TrophyMapFallback       string
	Places                  []places.Place
	Query                   string
	LocationQuery           string
	LocationStatus          string
	HasLocation             bool
	OriginLatitude          float64
	OriginLongitude         float64
	NowLocal                string
	PrefillName             string
	PrefillAddress          string
	PrefillCity             string
	PrefillLatitude         string
	PrefillLongitude        string
	PrefillPhone            string
	PrefillWebsite          string
	PrefillPlaceID          string
	PrefillGoogleRating     string
	PrefillGooglePriceLevel string
	PrefillCategory         string
	PrefillPriceLevel       int
	PrefillPickerID         string
	PrefillRestaurantID     string
	PrefillNotes            string
	PrefillNewTag           string
	PrefillIsChain          bool
	PrefillRatings          map[string]string
	PrefillTagIDs           map[string]bool
	PrefillPhotoDataURIs    []string
	PrefillFromPost         bool
	ReturnVisitID           string
}

type RestaurantResult struct {
	Restaurant    model.Restaurant
	LatestVisit   *model.Visit
	VisitCount    int
	AverageRating float64
	Tags          []model.Tag
}

type TrophyMapLabel struct {
	Name string
	Left string
	Top  string
}

func Render(w io.Writer, name string, data PageData) error {
	data.Title = title(data.Title)
	if data.NowLocal == "" {
		data.NowLocal = apptime.FormatDatetimeLocal(time.Now())
	}
	return pageTemplates.ExecuteTemplate(w, name, data)
}

func funcs() template.FuncMap {
	return template.FuncMap{
		"date": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return t.In(apptime.EasternLocation()).Format("Jan 2, 2006")
		},
		"avg": func(v model.Visit) string {
			if len(v.Ratings) == 0 {
				return "-"
			}
			var sum float64
			for _, rating := range v.Ratings {
				sum += rating.Score
			}
			return fmt.Sprintf("%.1f", sum/float64(len(v.Ratings)))
		},
		"dollars": func(n int) string {
			if n < 1 {
				return "-"
			}
			if n > 4 {
				n = 4
			}
			return strings.Repeat("$", n)
		},
		"score": func(n float64) string {
			if math.Mod(n, 1) == 0 {
				return fmt.Sprintf("%.0f", n)
			}
			return fmt.Sprintf("%.1f", n)
		},
		"str": func(value *string) string {
			if value == nil {
				return ""
			}
			return *value
		},
		"floatInput": func(value *float64) string {
			if value == nil {
				return ""
			}
			return strconv.FormatFloat(*value, 'f', -1, 64)
		},
		"intValue": func(value *int) int {
			if value == nil {
				return 0
			}
			return *value
		},
		"datetime": func(t time.Time) string {
			if t.IsZero() {
				return ""
			}
			return apptime.FormatDatetimeLocal(t)
		},
		"ratingValue": func(visit *model.Visit, person model.Person) string {
			if visit == nil {
				return ""
			}
			for _, rating := range visit.Ratings {
				if rating.Person.ID == person.ID {
					return strconv.FormatFloat(rating.Score, 'f', -1, 64)
				}
			}
			return ""
		},
		"hasVisitTag": func(visit *model.Visit, tag model.Tag) bool {
			if visit == nil {
				return false
			}
			for _, existing := range visit.Tags {
				if existing.ID == tag.ID {
					return true
				}
			}
			return false
		},
		"default": func(value, fallback string) string {
			if strings.TrimSpace(value) == "" {
				return fallback
			}
			return value
		},
		"query": func(value string) template.URL {
			return template.URL(strings.ReplaceAll(url.QueryEscape(value), "+", "%20"))
		},
		"imageSrc": func(value string) template.URL {
			trimmed := strings.TrimSpace(value)
			if !strings.HasPrefix(strings.ToLower(trimmed), "data:image/jpeg;base64,") {
				return ""
			}
			return template.URL(trimmed)
		},
		"asset": asset,
		"price": places.PriceLevelNumber,
		"distance": func(lat, lng float64, place places.Place) string {
			miles := places.DistanceMiles(lat, lng, place)
			if miles < 0.05 {
				return "<0.1 mi"
			}
			if miles < 10 {
				return fmt.Sprintf("%.1f mi", miles)
			}
			return fmt.Sprintf("%.0f mi", miles)
		},
		"avatar":             avatar,
		"placeCategory":      places.Category,
		"placeCity":          places.City,
		"prefillRatingValue": prefillRatingValue,
		"prefillTagChecked":  prefillTagChecked,
		"prefillHasRating":   prefillHasRating,
		"add1": func(value int) int {
			return value + 1
		},
		"maxVisitPhotos": func() int {
			return model.MaxVisitPhotos
		},
		"maxVisitPhotoBytes": func() int {
			return model.MaxVisitPhotoBytes
		},
	}
}

func prefillRatingValue(ratings map[string]string, id uuid.UUID) string {
	if ratings == nil {
		return ""
	}
	return ratings[id.String()]
}

func prefillTagChecked(tags map[string]bool, id uuid.UUID) bool {
	return tags != nil && tags[id.String()]
}

func prefillHasRating(ratings map[string]string) bool {
	for _, value := range ratings {
		rating, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err == nil && rating >= 0 && rating <= 10 && rating*2 == float64(int(rating*2)) {
			return true
		}
	}
	return false
}

// LoadAssetVersions records a ?v=<modification time> version for each file
// directly in dir, which is served at /static/. Call it once at startup so
// asset() can version URLs without touching the filesystem per request.
func LoadAssetVersions(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	versions := make(map[string]string, len(entries))
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		path := "/static/" + entry.Name()
		versions[path] = fmt.Sprintf("%s?v=%d", path, info.ModTime().Unix())
	}
	assetVersions.Store(&versions)
	return nil
}

// asset returns the versioned URL for a static file, or the path unchanged
// when it has no recorded version.
func asset(assetPath string) template.URL {
	if versions := assetVersions.Load(); versions != nil {
		if versioned, ok := (*versions)[assetPath]; ok {
			return template.URL(versioned)
		}
	}
	return template.URL(assetPath)
}

func avatar(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "daniel":
		return "/static/assets/dined-avatar-daniel.png"
	case "jen":
		return "/static/assets/dined-avatar-jennifer.png"
	case "caleb":
		return "/static/assets/dined-avatar-caleb.png"
	case "aiden":
		return "/static/assets/dined-avatar-aiden.png"
	default:
		return ""
	}
}

func title(value string) string {
	if value == "" {
		return "Dined"
	}
	return value + " - Dined"
}
