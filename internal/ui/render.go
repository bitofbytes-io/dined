package ui

import (
	"fmt"
	"html/template"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
)

type PageData struct {
	Title               string
	Authenticated       bool
	Error               string
	Visits              []model.Visit
	People              []model.Person
	Tags                []model.Tag
	Restaurants         []model.Restaurant
	SearchResults       []RestaurantResult
	Restaurant          *model.Restaurant
	Stats               model.Stats
	Places              []places.Place
	Query               string
	LocationQuery       string
	LocationStatus      string
	HasLocation         bool
	OriginLatitude      float64
	OriginLongitude     float64
	NowLocal            string
	PrefillName         string
	PrefillAddress      string
	PrefillPlaceID      string
	PrefillCategory     string
	PrefillPriceLevel   int
	PrefillRestaurantID string
}

type RestaurantResult struct {
	Restaurant    model.Restaurant
	LatestVisit   *model.Visit
	VisitCount    int
	AverageRating float64
	Tags          []model.Tag
}

func Render(w io.Writer, name string, data PageData) error {
	data.Title = title(data.Title)
	if data.NowLocal == "" {
		data.NowLocal = apptime.FormatDatetimeLocal(time.Now())
	}
	tpl, err := template.New("dined").Funcs(funcs()).Parse(templates)
	if err != nil {
		return err
	}
	return tpl.ExecuteTemplate(w, name, data)
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
		"default": func(value, fallback string) string {
			if strings.TrimSpace(value) == "" {
				return fallback
			}
			return value
		},
		"query": func(value string) template.URL {
			return template.URL(strings.ReplaceAll(url.QueryEscape(value), "+", "%20"))
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
		"avatar": avatar,
		"placeCategory": func(place places.Place) string {
			for _, typ := range place.Types {
				switch typ {
				case "american_restaurant":
					return "American"
				case "mexican_restaurant":
					return "Mexican"
				case "italian_restaurant":
					return "Italian"
				case "pizza_restaurant":
					return "Pizza"
				case "hamburger_restaurant":
					return "Burgers"
				case "breakfast_restaurant":
					return "Breakfast"
				case "chinese_restaurant":
					return "Chinese"
				case "japanese_restaurant":
					return "Japanese"
				case "thai_restaurant":
					return "Thai"
				case "indian_restaurant":
					return "Indian"
				case "barbecue_restaurant":
					return "BBQ"
				case "seafood_restaurant":
					return "Seafood"
				case "dessert_restaurant", "ice_cream_shop", "bakery":
					return "Dessert"
				case "cafe", "coffee_shop":
					return "Coffee"
				}
			}
			return ""
		},
	}
}

func asset(assetPath string) template.URL {
	const staticPrefix = "/static/"
	if !strings.HasPrefix(assetPath, staticPrefix) {
		return template.URL(assetPath)
	}

	rel := strings.TrimPrefix(assetPath, staticPrefix)
	filePath := filepath.Join("static", filepath.FromSlash(rel))
	staticRoot, err := filepath.Abs("static")
	if err != nil {
		return template.URL(assetPath)
	}
	candidate, err := filepath.Abs(filePath)
	if err != nil {
		return template.URL(assetPath)
	}
	if candidate != staticRoot && !strings.HasPrefix(candidate, staticRoot+string(os.PathSeparator)) {
		return template.URL(assetPath)
	}

	info, err := os.Stat(candidate)
	if err != nil {
		return template.URL(assetPath)
	}
	return template.URL(fmt.Sprintf("%s?v=%d", assetPath, info.ModTime().Unix()))
}

func avatar(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "daniel":
		return "/static/assets/dined-avatar-daniel.png"
	case "jennifer":
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

const templates = `
{{define "top"}}
<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="theme-color" content="#0d6f6f">
  <title>{{.Title}}</title>
  <link rel="manifest" href="/site.webmanifest?v=20260513">
  <link rel="icon" href="/favicon.ico?v=20260513" sizes="any">
  <link rel="apple-touch-icon" href="/apple-touch-icon.png?v=20260513">
  <link rel="icon" type="image/png" sizes="32x32" href="/favicon-32x32.png?v=20260513">
  <link rel="icon" type="image/png" sizes="16x16" href="/favicon-16x16.png?v=20260513">
  <link rel="stylesheet" href="{{asset "/static/styles.css"}}">
  <script src="{{asset "/static/htmx.min.js"}}" defer></script>
</head>
	<body hx-boost="true">
	  <div class="app-shell">
	    <header class="topbar">
	      <a class="brand" href="/" aria-label="Dined home"><img src="/static/assets/dined-logo-generated.png" alt="Dined"></a>
	      <button class="nav-toggle" type="button" aria-controls="primary-nav" aria-expanded="false" onclick="var h=this.closest('.topbar'); var open=h.classList.toggle('nav-open'); this.setAttribute('aria-expanded', open ? 'true' : 'false');"><span></span><span></span><span></span><span class="sr-only">Menu</span></button>
	      <nav class="nav" id="primary-nav" aria-label="Primary">
	        <a href="/dines">Dines</a>
	        {{if .Authenticated}}<a href="/nearby">Nearby</a><a href="/search">Search</a><a href="/log">Log</a>{{end}}
	        <a href="/trophy-case">Trophy Case</a>
	        {{if .Authenticated}}<form method="post" action="/logout"><button class="link-button">LOGOUT</button></form>{{else}}<a href="/login">Login</a>{{end}}
      </nav>
    </header>
    {{if .Error}}<div class="flash" role="alert">{{.Error}}</div>{{end}}
{{end}}

{{define "bottom"}}
	  </div>
	  <script>
	    function dinedRestaurantOptions(value) {
	      var options = document.querySelectorAll("#restaurant-options option");
	      var matches = [];
	      for (var i = 0; i < options.length; i += 1) {
	        if (options[i].value === value) matches.push(options[i]);
	      }
	      return matches;
	    }

	    function dinedSyncRestaurantSelection(form) {
	      if (!form) return;
	      var name = form.querySelector("input[name='restaurant_name']");
	      var id = form.querySelector("input[name='restaurant_id']");
	      var address = form.querySelector("input[name='address']");
	      var place = form.querySelector("input[name='google_place_id']");
	      var category = form.querySelector("select[name='category']");
	      if (!name || !id) return;

	      var options = dinedRestaurantOptions(name.value);
	      if (!options.length) {
	        id.value = "";
	        return;
	      }
	      var option = null;
	      var currentAddress = address ? address.value.trim() : "";
	      if (currentAddress) {
	        for (var i = 0; i < options.length; i += 1) {
	          if ((options[i].dataset.address || "").trim() === currentAddress) {
	            option = options[i];
	            break;
	          }
	        }
	      } else if (options.length === 1) {
	        option = options[0];
	      }
	      if (!option) {
	        id.value = "";
	        return;
	      }
	      var optionAddress = option.dataset.address || "";
	      if (address && address.value && optionAddress && address.value !== optionAddress) {
	        id.value = "";
	        return;
	      }

	      id.value = option.dataset.restaurantId || "";
	      if (address && !address.value) address.value = optionAddress;
	      if (place && !place.value) place.value = option.dataset.googlePlaceId || "";
	      if (category && !category.value) category.value = option.dataset.category || "";
	    }

	    document.addEventListener("input", function (event) {
	      if (event.target.name === "restaurant_name" || event.target.name === "address") {
	        dinedSyncRestaurantSelection(event.target.form);
	      }
	      if (event.target.matches("[data-half-step-rating]")) {
	        dinedValidateHalfStepRating(event.target);
	      }
	    });

	    function dinedValidateHalfStepRating(input) {
	      if (!input || input.value === "") {
	        input.setCustomValidity("");
	        return;
	      }
	      var rating = Number(input.value);
	      var isHalfStep = Number.isFinite(rating) && rating >= 0 && rating <= 10 && Number.isInteger(rating * 2);
	      input.setCustomValidity(isHalfStep ? "" : "Use whole numbers or .5 increments from 0 to 10.");
	    }

	    document.addEventListener("DOMContentLoaded", function () {
	      var ratings = document.querySelectorAll("[data-half-step-rating]");
	      for (var i = 0; i < ratings.length; i += 1) {
	        dinedValidateHalfStepRating(ratings[i]);
	      }
	    });

	    document.addEventListener("click", function (event) {
	      if (event.target.id !== "use-location") return;
	      var button = event.target;
	      var form = document.getElementById("nearby-form");
	      var status = document.getElementById("location-status");
	      var blockedMessage = "Location is blocked before Dined can ask. Enable location for this browser/site in system or browser settings, then reload.";
	      if (!navigator.geolocation) {
	        if (status) status.textContent = "Browser location is not available. Search by city, address, or neighborhood instead.";
	        return;
	      }

	      function requestBrowserLocation() {
	        if (status) status.textContent = "Finding restaurants near you...";
	        button.disabled = true;
	        navigator.geolocation.getCurrentPosition(function (position) {
	          var lat = form ? form.querySelector("input[name='lat']") : null;
	          var lng = form ? form.querySelector("input[name='lng']") : null;
	          if (lat) lat.value = position.coords.latitude.toFixed(6);
	          if (lng) lng.value = position.coords.longitude.toFixed(6);
	          if (form) form.submit();
	        }, function (error) {
	          button.disabled = false;
	          if (!status) return;
	          if (error && error.code === error.PERMISSION_DENIED) {
	            status.textContent = blockedMessage;
	          } else if (error && error.code === error.TIMEOUT) {
	            status.textContent = "Location timed out. Try again, or search by city/address.";
	          } else {
	            status.textContent = "Location was not shared. Search by city, address, or neighborhood instead.";
	          }
	        }, { enableHighAccuracy: true, timeout: 10000, maximumAge: 300000 });
	      }

	      if (navigator.permissions && navigator.permissions.query) {
	        navigator.permissions.query({ name: "geolocation" }).then(function (permission) {
	          if (permission.state === "denied") {
	            if (status) status.textContent = blockedMessage;
	            return;
	          }
	          requestBrowserLocation();
	        }).catch(requestBrowserLocation);
	      } else {
	        requestBrowserLocation();
	      }
	    });

    document.addEventListener("submit", function (event) {
      var form = event.target;
      if (!form || form.id !== "nearby-form") return;
      var lat = form.querySelector("input[name='lat']");
      var lng = form.querySelector("input[name='lng']");
      var near = form.querySelector("input[name='near']");
      if (!lat || !lng || (lat.value.trim() && lng.value.trim()) || (near && near.value.trim())) return;
      event.preventDefault();
      var useLocation = document.getElementById("use-location");
      if (useLocation) useLocation.click();
    });
  </script>
</body>
</html>
{{end}}

{{define "visit-list"}}
{{if .Visits}}
  <div class="visit-list">
  {{range .Visits}}
    <article class="visit-card" id="{{.ID}}">
      <div class="visit-main">
        <h3><a href="/restaurants/{{.Restaurant.ID}}">{{.Restaurant.Name}}</a>{{if .Restaurant.IsChain}} <span class="badge">Chain</span>{{end}}</h3>
        <p class="visit-meta">{{date .VisitedAt}} · Picked by {{.Picker.Name}} · {{dollars .PriceLevel}}</p>
        {{if .Restaurant.Address}}<p class="muted">{{.Restaurant.Address}}</p>{{end}}
      </div>
      <div class="score-disc">{{avg .}}</div>
      <div class="ratings">{{range .Ratings}}<span>{{.Person.Name}} {{score .Score}}</span>{{end}}</div>
      {{if .Tags}}<div class="tags">{{range .Tags}}<span>{{.Name}}</span>{{end}}</div>{{end}}
      {{if .Notes}}<p class="note">{{.Notes}}</p>{{end}}
      {{if $.Authenticated}}<form method="post" action="/visits/{{.ID}}/delete" onsubmit="return confirm('Delete this dine?')"><button class="danger">Delete</button></form>{{end}}
    </article>
  {{end}}
  </div>
{{else}}
  <p class="empty">No dines logged yet.</p>
{{end}}
{{end}}

{{define "home"}}
{{template "top" .}}
	<main class="booth-stage">
	  <section class="booth-scene" aria-label="Dined booth interior">
	    <div class="booth-layer booth-layer-action">
      <div class="booth-search">
        <h1>Have we eaten here before?</h1>
        {{if .Authenticated}}
        <form action="/search" method="get" class="search-row">
          <input name="q" type="search" placeholder="Search restaurants..." value="{{.Query}}">
          <button>Search</button>
        </form>
        {{else}}
        <p>Log in to search Google Places or add new dines.</p>
        {{end}}
      </div>

      {{if .Authenticated}}<a class="image-cta" href="/log"><img src="/static/assets/dined-log-arrow-generated.png" alt="Log a Dine"></a>{{else}}<a class="public-badge-link" href="/dines"><span>View Dines</span></a>{{end}}
    </div>

    <div class="booth-recent">
      <div class="section-head"><h2>Recent Dines</h2><a href="/dines">View all</a></div>
      {{template "visit-list" .}}
    </div>
  </section>
</main>
{{template "bottom" .}}
{{end}}

{{define "dines"}}
{{template "top" .}}
<main class="page-band ledger-page">
  <section class="wide-ticket ledger-panel">
	    <div class="section-head"><div><h1>All Dines</h1></div>{{if .Authenticated}}<a class="small-cta" href="/log">Log a Dine</a>{{end}}</div>
    {{template "visit-list" .}}
  </section>
</main>
{{template "bottom" .}}
{{end}}

{{define "log"}}
{{template "top" .}}
<main class="page-band order-page">
	<form class="wide-ticket log-form order-pad console-pad" method="post" action="/visits">
	    <div class="section-head console-head">
		      <div><h1>Log a Dine</h1></div>
	    </div>
    {{if .PrefillPlaceID}}<p class="place-source">Google place loaded: {{.PrefillName}}</p>{{end}}
    <div class="console-grid">
	      <section class="form-section restaurant-console">
	        <input type="hidden" name="restaurant_id" value="{{.PrefillRestaurantID}}">
	        <div class="form-grid restaurant-grid">
	          <label>Restaurant<input name="restaurant_name" list="restaurant-options" placeholder="Search or add restaurant" value="{{.PrefillName}}"><datalist id="restaurant-options">{{range .Restaurants}}<option value="{{.Name}}" data-restaurant-id="{{.ID}}" data-address="{{if .Address}}{{.Address}}{{end}}" data-google-place-id="{{if .GooglePlaceID}}{{.GooglePlaceID}}{{end}}" data-category="{{if .Category}}{{.Category}}{{end}}">{{if .Address}}{{.Address}}{{end}}</option>{{end}}</datalist></label>
	          <label>Address<input name="address" placeholder="Optional" value="{{.PrefillAddress}}"></label>
          <label>Google Place ID<input name="google_place_id" placeholder="Optional" value="{{.PrefillPlaceID}}"></label>
          <label>Category<select name="category"><option></option><option {{if eq .PrefillCategory "American"}}selected{{end}}>American</option><option {{if eq .PrefillCategory "Mexican"}}selected{{end}}>Mexican</option><option {{if eq .PrefillCategory "Italian"}}selected{{end}}>Italian</option><option {{if eq .PrefillCategory "Pizza"}}selected{{end}}>Pizza</option><option {{if eq .PrefillCategory "Burgers"}}selected{{end}}>Burgers</option><option {{if eq .PrefillCategory "Breakfast"}}selected{{end}}>Breakfast</option><option {{if eq .PrefillCategory "Chinese"}}selected{{end}}>Chinese</option><option {{if eq .PrefillCategory "Japanese"}}selected{{end}}>Japanese</option><option {{if eq .PrefillCategory "Thai"}}selected{{end}}>Thai</option><option {{if eq .PrefillCategory "Indian"}}selected{{end}}>Indian</option><option {{if eq .PrefillCategory "BBQ"}}selected{{end}}>BBQ</option><option {{if eq .PrefillCategory "Seafood"}}selected{{end}}>Seafood</option><option {{if eq .PrefillCategory "Dessert"}}selected{{end}}>Dessert</option><option {{if eq .PrefillCategory "Coffee"}}selected{{end}}>Coffee</option><option {{if eq .PrefillCategory "Other"}}selected{{end}}>Other</option></select></label>
        </div>
      </section>
      <section class="form-section visit-console">
        <div class="form-grid visit-form-grid">
          <label>Date and time<input type="datetime-local" name="visited_at" value="{{.NowLocal}}"></label>
          <label>Picked by<select name="picker_id" required>{{range .People}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select></label>
          <label>Price Level<select name="price_level"><option value="1" {{if eq .PrefillPriceLevel 1}}selected{{end}}>$</option><option value="2" {{if or (eq .PrefillPriceLevel 0) (eq .PrefillPriceLevel 2)}}selected{{end}}>$$</option><option value="3" {{if eq .PrefillPriceLevel 3}}selected{{end}}>$$$</option><option value="4" {{if eq .PrefillPriceLevel 4}}selected{{end}}>$$$$</option></select></label>
        </div>
      </section>
    </div>
	    <fieldset class="ratings-field score-console"><legend>Rate Your Experience</legend>{{range .People}}<label class="rating-card">{{if avatar .Name}}<img class="avatar-face" src="{{avatar .Name}}" alt="">{{else}}<span class="avatar-dot">{{slice .Name 0 1}}</span>{{end}}<span>{{.Name}}</span><input name="rating_{{.ID}}" type="number" min="0" max="10" step="0.5" inputmode="decimal" placeholder="0-10" data-half-step-rating></label>{{end}}</fieldset>
    <fieldset class="tags-field chip-field"><legend>Tags</legend>{{range .Tags}}<label><input type="checkbox" name="tag_id" value="{{.ID}}"> <span>{{.Name}}</span></label>{{end}}<label class="new-tag">New tag<input name="new_tag" placeholder="Great fries"></label></fieldset>
    <label class="notes-field">Notes<textarea name="notes" rows="4" placeholder="What should we remember next time?"></textarea></label>
    <div class="form-actions console-actions"><label class="inline-check"><input type="checkbox" name="is_chain" value="true"> Mark new restaurant as chain</label><button class="primary-button">Save Dine</button></div>
  </form>
</main>
{{template "bottom" .}}
{{end}}

{{define "restaurant"}}
{{template "top" .}}
<main class="page-band ledger-page">
  <section class="wide-ticket ledger-panel">
    {{with .Restaurant}}
    <div class="section-head"><div><h1>{{.Name}} {{if .IsChain}}<span class="badge">Chain</span>{{end}}</h1>{{if .Address}}<p>{{.Address}}</p>{{end}}</div>{{if $.Authenticated}}<a class="small-cta" href="/log?restaurant_id={{.ID}}&restaurant_name={{query .Name}}{{if .Address}}&address={{query .Address}}{{end}}{{if .GooglePlaceID}}&google_place_id={{query .GooglePlaceID}}{{end}}{{if .Category}}&category={{query .Category}}{{end}}">Log Another Dine</a>{{end}}</div>
    <dl class="details"><dt>Category</dt><dd>{{if .Category}}{{.Category}}{{else}}-{{end}}</dd><dt>Phone</dt><dd>{{if .Phone}}{{.Phone}}{{else}}-{{end}}</dd><dt>Website</dt><dd>{{if .Website}}<a href="{{.Website}}">{{.Website}}</a>{{else}}-{{end}}</dd><dt>Google rating</dt><dd>{{if .GoogleRating}}{{.GoogleRating}}{{else}}-{{end}}</dd></dl>
    {{if $.Authenticated}}<form method="post" action="/restaurants/{{.ID}}/chain" class="inline-form"><input type="hidden" name="is_chain" value="{{if .IsChain}}false{{else}}true{{end}}"><button class="small-cta">{{if .IsChain}}Clear Chain Badge{{else}}Mark Chain{{end}}</button></form>{{end}}
    {{end}}
    {{template "visit-list" .}}
  </section>
</main>
{{template "bottom" .}}
{{end}}

{{define "trophy"}}
{{template "top" .}}
<main class="jukebox-stage">
  <section class="trophy-panel">
    <div class="trophy-title">
      <p>Dined Family Soundtrack</p>
      <h1>Trophy Case</h1>
    </div>
    <div class="record-grid" aria-label="Dined family records">
      <div class="record"><span>{{.Stats.TotalDines}}</span><p>Total Dines</p></div>
      <div class="record"><span>{{printf "%.1f" .Stats.AverageRating}}</span><p>Family Average</p></div>
      <div class="award"><h2>Safe Bet</h2><p>{{default .Stats.HighestRatedRestaurant "Waiting on more dines"}}</p></div>
      <div class="award"><h2>The Regular</h2><p>{{default .Stats.MostVisitedRestaurant "Waiting on more dines"}}</p></div>
      <div class="award"><h2>Best Picker</h2><p>{{default .Stats.BestPicker "Waiting on more dines"}}</p></div>
      <div class="award"><h2>Table Divided</h2><p>{{default .Stats.BiggestSplitRestaurant "Waiting on more dines"}}</p></div>
    </div>
    <div class="track-list">
      <h2>Track List</h2>
      <p>Best Picker <span>{{default .Stats.BestPicker "Waiting on more dines"}}</span></p>
      <p>Safe Bet <span>{{default .Stats.HighestRatedRestaurant "Waiting on more dines"}}</span></p>
      <p>The Regular <span>{{default .Stats.MostVisitedRestaurant "Waiting on more dines"}}</span></p>
      <p>Table Divided <span>{{default .Stats.BiggestSplitRestaurant "Waiting on more dines"}}</span></p>
    </div>
  </section>
</main>
{{template "bottom" .}}
{{end}}

{{define "login"}}
{{template "top" .}}
<main class="page-band pass-page"><form class="login-card kitchen-pass" method="post" action="/login"><p class="eyebrow">Private Counter</p><h1>Kitchen Pass</h1><label>API token<input type="password" name="token" autofocus></label><input type="hidden" name="redirect" value="{{.Query}}"><button class="primary-button">Enter</button></form></main>
{{template "bottom" .}}
{{end}}

{{define "search"}}
{{template "top" .}}
<main class="page-band search-page">
  <section class="wide-ticket sign-panel search-panel">
    <p class="eyebrow">Counter Search</p>
    <h1>Have we eaten here before?</h1>
    <form class="search-row" method="get" action="/search"><input name="q" type="search" value="{{.Query}}" placeholder="Restaurant name"><button>Search</button></form>
    {{if .SearchResults}}<h2>Saved Spots</h2><div class="restaurant-list history-list">{{range .SearchResults}}<a class="restaurant-row history-row" href="/restaurants/{{.Restaurant.ID}}"><div><strong>{{.Restaurant.Name}}</strong>{{if .Restaurant.IsChain}}<em>Chain</em>{{end}}</div><span>{{if .Restaurant.Address}}{{.Restaurant.Address}}{{end}}</span>{{with .LatestVisit}}<span>Last visit {{date .VisitedAt}} · Picked by {{.Picker.Name}} · {{dollars .PriceLevel}}</span>{{end}}<div class="row-stats"><span>{{.VisitCount}} {{if eq .VisitCount 1}}visit{{else}}visits{{end}}</span><span>Avg {{score .AverageRating}}</span></div>{{if .Tags}}<div class="tags">{{range .Tags}}<span>{{.Name}}</span>{{end}}</div>{{end}}</a>{{end}}</div>{{else if .Query}}<p class="empty">No saved dines match that search yet.</p>{{end}}
    {{if .Places}}<h2>Around Town</h2><div class="restaurant-list places-list">{{range .Places}}<article class="restaurant-row place-row"><div><strong>{{.DisplayName.Text}}</strong>{{if .Rating}}<em>{{score .Rating}} Google</em>{{end}}</div><span>{{.Address}}</span><span>{{if price .PriceLevel}}{{dollars (price .PriceLevel)}}{{else}}Price not listed{{end}}</span>{{if $.Authenticated}}<a class="small-cta" href="/log?restaurant_name={{query .DisplayName.Text}}&address={{query .Address}}&google_place_id={{query .ID}}&category={{query (placeCategory .)}}&price_level={{price .PriceLevel}}">Log this dine</a>{{end}}</article>{{end}}</div>{{else if .Query}}<p class="empty">Google Places results will appear here when a Places API key is configured.</p>{{end}}
  </section>
</main>
{{template "bottom" .}}
{{end}}

{{define "nearby"}}
{{template "top" .}}
<main class="page-band nearby-page">
  <section class="wide-ticket sign-panel nearby-panel">
    <p class="eyebrow">Roadside Finder</p>
    <h1>Nearby</h1>
    <form class="nearby-controls" id="nearby-form" method="get" action="/nearby"><input type="hidden" name="lat"><input type="hidden" name="lng"><div class="nearby-search-line"><input name="q" type="search" value="{{.Query}}" placeholder="Restaurant or cuisine" aria-label="Restaurant or cuisine"><button class="primary-button" id="use-location" type="button">Search Near Me</button></div><div class="nearby-search-line nearby-fallback-line"><input name="near" type="search" value="{{.LocationQuery}}" placeholder="City, address, or neighborhood" aria-label="City, address, or neighborhood"><button class="secondary-button">Search This Area</button></div></form>
    <p class="empty" id="location-status">{{if .LocationStatus}}{{.LocationStatus}}{{else}}Share your browser location, or search near a city, address, or neighborhood.{{end}}</p>
    {{if .Places}}<div class="restaurant-list places-list">{{range .Places}}<article class="restaurant-row place-row"><div><strong>{{.DisplayName.Text}}</strong>{{if .Rating}}<em>{{score .Rating}} Google</em>{{end}}</div><span>{{.Address}}</span><span>{{if $.HasLocation}}{{distance $.OriginLatitude $.OriginLongitude .}} · {{end}}{{if price .PriceLevel}}{{dollars (price .PriceLevel)}}{{else}}Price not listed{{end}}</span>{{if $.Authenticated}}<a class="small-cta" href="/log?restaurant_name={{query .DisplayName.Text}}&address={{query .Address}}&google_place_id={{query .ID}}&category={{query (placeCategory .)}}&price_level={{price .PriceLevel}}">Log this dine</a>{{end}}</article>{{end}}</div>{{end}}
  </section>
</main>
{{template "bottom" .}}
{{end}}
`
