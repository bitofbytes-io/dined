package ui

import (
	"fmt"
	"html/template"
	"io"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
)

type PageData struct {
	Title             string
	Authenticated     bool
	Error             string
	Visits            []model.Visit
	People            []model.Person
	Tags              []model.Tag
	Restaurants       []model.Restaurant
	SearchResults     []RestaurantResult
	Restaurant        *model.Restaurant
	Stats             model.Stats
	Places            []places.Place
	Query             string
	NowLocal          string
	PrefillName       string
	PrefillAddress    string
	PrefillPlaceID    string
	PrefillCategory   string
	PrefillPriceLevel int
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
		data.NowLocal = time.Now().Format("2006-01-02T15:04")
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
			return t.Format("Jan 2, 2006")
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
		"query":  url.QueryEscape,
		"price":  places.PriceLevelNumber,
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
  <link rel="manifest" href="/site.webmanifest">
  <link rel="apple-touch-icon" href="/apple-touch-icon.png">
  <link rel="icon" type="image/png" sizes="32x32" href="/favicon-32x32.png">
  <link rel="icon" type="image/png" sizes="16x16" href="/favicon-16x16.png">
  <link rel="stylesheet" href="/static/styles.css">
  <script src="/static/htmx.min.js" defer></script>
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
	    document.addEventListener("click", function (event) {
	      if (event.target.id !== "use-location") return;
	      var status = document.getElementById("location-status");
      if (!navigator.geolocation) {
        if (status) status.textContent = "Browser location is not available. Enter coordinates instead.";
        return;
      }
      if (status) status.textContent = "Finding restaurants near you...";
      navigator.geolocation.getCurrentPosition(function (position) {
        var lat = document.querySelector("input[name='lat']");
        var lng = document.querySelector("input[name='lng']");
        if (lat) lat.value = position.coords.latitude.toFixed(6);
        if (lng) lng.value = position.coords.longitude.toFixed(6);
        var form = document.getElementById("nearby-form");
        if (form) form.submit();
      }, function () {
        if (status) status.textContent = "Location was not shared. Enter latitude and longitude instead.";
      }, { enableHighAccuracy: true, timeout: 10000, maximumAge: 300000 });
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
        <p>The ledger is public. Log in to search Google Places or add new dines.</p>
        {{end}}
      </div>

      {{if .Authenticated}}<a class="image-cta" href="/log"><img src="/static/assets/dined-log-arrow-generated.png" alt="Log a Dine"></a>{{else}}<a class="public-badge-link" href="/dines"><img src="/static/assets/dined-public-ledger-badge.png" alt="Public Ledger"></a>{{end}}
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
	        <div class="form-grid restaurant-grid">
	          <label>Restaurant<input name="restaurant_name" list="restaurant-options" placeholder="Type 3+ characters or add a new restaurant" value="{{.PrefillName}}"><datalist id="restaurant-options">{{range .Restaurants}}<option value="{{.Name}}">{{if .Address}}{{.Address}}{{end}}</option>{{end}}</datalist></label>
	          <label>Address<input name="address" placeholder="Optional" value="{{.PrefillAddress}}"></label>
          <label>Google Place ID<input name="google_place_id" placeholder="Optional" value="{{.PrefillPlaceID}}"></label>
          <label>Category<select name="category"><option></option><option {{if eq .PrefillCategory "American"}}selected{{end}}>American</option><option {{if eq .PrefillCategory "Mexican"}}selected{{end}}>Mexican</option><option {{if eq .PrefillCategory "Italian"}}selected{{end}}>Italian</option><option {{if eq .PrefillCategory "Pizza"}}selected{{end}}>Pizza</option><option {{if eq .PrefillCategory "Burgers"}}selected{{end}}>Burgers</option><option {{if eq .PrefillCategory "Breakfast"}}selected{{end}}>Breakfast</option><option {{if eq .PrefillCategory "Chinese"}}selected{{end}}>Chinese</option><option {{if eq .PrefillCategory "Japanese"}}selected{{end}}>Japanese</option><option {{if eq .PrefillCategory "Thai"}}selected{{end}}>Thai</option><option {{if eq .PrefillCategory "Indian"}}selected{{end}}>Indian</option><option {{if eq .PrefillCategory "BBQ"}}selected{{end}}>BBQ</option><option {{if eq .PrefillCategory "Seafood"}}selected{{end}}>Seafood</option><option {{if eq .PrefillCategory "Dessert"}}selected{{end}}>Dessert</option><option {{if eq .PrefillCategory "Coffee"}}selected{{end}}>Coffee</option><option {{if eq .PrefillCategory "Other"}}selected{{end}}>Other</option></select></label>
        </div>
      </section>
      <section class="form-section visit-console">
        <h2>Visit</h2>
        <div class="form-grid visit-form-grid">
          <label>Date and time<input type="datetime-local" name="visited_at" value="{{.NowLocal}}"></label>
          <label>Picked by<select name="picker_id" required>{{range .People}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select></label>
        </div>
	        <fieldset class="price-field"><legend>Price Level</legend><label><input type="radio" name="price_level" value="1" {{if eq .PrefillPriceLevel 1}}checked{{end}}> <span>$</span></label><label><input type="radio" name="price_level" value="2" {{if or (eq .PrefillPriceLevel 0) (eq .PrefillPriceLevel 2)}}checked{{end}}> <span>$$</span></label><label><input type="radio" name="price_level" value="3" {{if eq .PrefillPriceLevel 3}}checked{{end}}> <span>$$$</span></label><label><input type="radio" name="price_level" value="4" {{if eq .PrefillPriceLevel 4}}checked{{end}}> <span>$$$$</span></label></fieldset>
      </section>
    </div>
	    <fieldset class="ratings-field score-console"><legend>Rate Your Experience</legend>{{range .People}}<label class="rating-card">{{if avatar .Name}}<img class="avatar-face" src="{{avatar .Name}}" alt="">{{else}}<span class="avatar-dot">{{slice .Name 0 1}}</span>{{end}}<span>{{.Name}}</span><input name="rating_{{.ID}}" type="number" min="0" max="10" step="0.5" placeholder="0-10"></label>{{end}}</fieldset>
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
    <div class="section-head"><div><h1>{{.Name}} {{if .IsChain}}<span class="badge">Chain</span>{{end}}</h1>{{if .Address}}<p>{{.Address}}</p>{{end}}</div>{{if $.Authenticated}}<a class="small-cta" href="/log">Log Another Dine</a>{{end}}</div>
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
    <p class="eyebrow">Search the Ledger</p>
    <h1>Have we eaten here before?</h1>
    <form class="search-row" method="get" action="/search"><input name="q" type="search" value="{{.Query}}" placeholder="Restaurant name"><button>Search</button></form>
    {{if .SearchResults}}<h2>Known Places</h2><div class="restaurant-list history-list">{{range .SearchResults}}<a class="restaurant-row history-row" href="/restaurants/{{.Restaurant.ID}}"><div><strong>{{.Restaurant.Name}}</strong>{{if .Restaurant.IsChain}}<em>Chain</em>{{end}}</div><span>{{if .Restaurant.Address}}{{.Restaurant.Address}}{{end}}</span>{{with .LatestVisit}}<span>Last visit {{date .VisitedAt}} · Picked by {{.Picker.Name}} · {{dollars .PriceLevel}}</span>{{end}}<div class="row-stats"><span>{{.VisitCount}} {{if eq .VisitCount 1}}visit{{else}}visits{{end}}</span><span>Avg {{score .AverageRating}}</span></div>{{if .Tags}}<div class="tags">{{range .Tags}}<span>{{.Name}}</span>{{end}}</div>{{end}}</a>{{end}}</div>{{else if .Query}}<p class="empty">No saved dines match that search yet.</p>{{end}}
    {{if .Places}}<h2>Google Places</h2><div class="restaurant-list places-list">{{range .Places}}<article class="restaurant-row place-row"><div><strong>{{.DisplayName.Text}}</strong>{{if .Rating}}<em>{{score .Rating}} Google</em>{{end}}</div><span>{{.Address}}</span><span>{{if .PriceLevel}}{{.PriceLevel}}{{else}}Price not listed{{end}}</span>{{if $.Authenticated}}<a class="small-cta" href="/log?restaurant_name={{query .DisplayName.Text}}&address={{query .Address}}&google_place_id={{query .ID}}&category={{query (placeCategory .)}}&price_level={{price .PriceLevel}}">Log this dine</a>{{end}}</article>{{end}}</div>{{else if .Query}}<p class="empty">Google Places results will appear here when a Places API key is configured.</p>{{end}}
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
    <form class="search-row nearby-controls" id="nearby-form" method="get" action="/nearby"><input name="lat" placeholder="Latitude"><input name="lng" placeholder="Longitude"><button>Find Restaurants</button><button class="secondary-button" id="use-location" type="button">Use Current Location</button></form>
    <p class="empty" id="location-status">Use your browser location or enter coordinates to search nearby restaurants.</p>
    {{if .Places}}<div class="restaurant-list places-list">{{range .Places}}<article class="restaurant-row place-row"><div><strong>{{.DisplayName.Text}}</strong>{{if .Rating}}<em>{{score .Rating}} Google</em>{{end}}</div><span>{{.Address}}</span><span>{{if .PriceLevel}}{{.PriceLevel}}{{else}}Price not listed{{end}}</span>{{if $.Authenticated}}<a class="small-cta" href="/log?restaurant_name={{query .DisplayName.Text}}&address={{query .Address}}&google_place_id={{query .ID}}&category={{query (placeCategory .)}}&price_level={{price .PriceLevel}}">Log this dine</a>{{end}}</article>{{end}}</div>{{end}}
  </section>
</main>
{{template "bottom" .}}
{{end}}
`
