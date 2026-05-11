package ui

import (
	"fmt"
	"html/template"
	"io"
	"math"
	"strings"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
)

type PageData struct {
	Title         string
	Authenticated bool
	Error         string
	Visits        []model.Visit
	People        []model.Person
	Tags          []model.Tag
	Restaurants   []model.Restaurant
	Restaurant    *model.Restaurant
	Stats         model.Stats
	Places        []places.Place
	Query         string
	NowLocal      string
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
			if n > 5 {
				n = 5
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
      <a class="brand" href="/" aria-label="Dined home">Dined</a>
      <nav class="nav" aria-label="Primary">
        <a href="/dines">Dines</a>
        {{if .Authenticated}}<a href="/nearby">Nearby</a><a href="/search">Search</a><a href="/log">Log</a>{{end}}
        <a href="/trophy-case">Trophy Case</a>
        {{if .Authenticated}}<form method="post" action="/logout"><button class="link-button">Logout</button></form>{{else}}<a href="/login">Login</a>{{end}}
      </nav>
    </header>
    {{if .Error}}<div class="flash" role="alert">{{.Error}}</div>{{end}}
{{end}}

{{define "bottom"}}
  </div>
</body>
</html>
{{end}}

{{define "visit-list"}}
{{if .Visits}}
  <div class="visit-list">
  {{range .Visits}}
    <article class="visit-card" id="{{.ID}}">
      <div>
        <h3><a href="/restaurants/{{.Restaurant.ID}}">{{.Restaurant.Name}}</a>{{if .Restaurant.IsChain}} <span class="badge">Chain</span>{{end}}</h3>
        <p>{{date .VisitedAt}} · Picked by {{.Picker.Name}} · {{dollars .PriceLevel}}</p>
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
    <div class="booth-logo">
      <img src="/static/assets/dined-logo-sign.png" alt="Dined">
      <p>Proof that nobody actually agreed on dinner.</p>
    </div>

    <div class="booth-family">
      <p>The Dined Family</p>
      <div><span>Daniel</span><span>Jennifer</span><span>Caleb</span><span>Aiden</span></div>
    </div>

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

    {{if .Authenticated}}<a class="image-cta" href="/log"><img src="/static/assets/dined-log-arrow.png" alt="Log a Dine"></a>{{else}}<a class="public-badge-link" href="/dines"><img src="/static/assets/dined-public-ledger-badge.png" alt="Public Ledger"></a>{{end}}

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
<main class="page-band">
  <section class="wide-ticket">
    <div class="section-head"><h1>All Dines</h1>{{if .Authenticated}}<a class="small-cta" href="/log">Log a Dine</a>{{end}}</div>
    {{template "visit-list" .}}
  </section>
</main>
{{template "bottom" .}}
{{end}}

{{define "log"}}
{{template "top" .}}
<main class="page-band">
  <form class="wide-ticket log-form" method="post" action="/visits">
    <h1>Log a Dine</h1>
    <div class="form-grid">
      <label>Known restaurant<select name="restaurant_id"><option value="">Add a new restaurant</option>{{range .Restaurants}}<option value="{{.ID}}">{{.Name}}{{if .Address}} - {{.Address}}{{end}}</option>{{end}}</select></label>
      <label>New restaurant name<input name="restaurant_name" placeholder="Hank's Downtown Diner"></label>
      <label>Address<input name="address" placeholder="Optional"></label>
      <label>Google Place ID<input name="google_place_id" placeholder="Optional"></label>
      <label>Category<select name="category"><option></option><option>American</option><option>Mexican</option><option>Italian</option><option>Pizza</option><option>Burgers</option><option>Breakfast</option><option>Chinese</option><option>Japanese</option><option>Thai</option><option>Indian</option><option>BBQ</option><option>Seafood</option><option>Dessert</option><option>Coffee</option><option>Other</option></select></label>
      <label>Date and time<input type="datetime-local" name="visited_at" value="{{.NowLocal}}"></label>
      <label>Picked by<select name="picker_id" required>{{range .People}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select></label>
      <label>Price level<select name="price_level"><option value="1">$</option><option value="2" selected>$$</option><option value="3">$$$</option><option value="4">$$$$</option><option value="5">$$$$$</option></select></label>
    </div>
    <fieldset class="ratings-field"><legend>Family ratings</legend>{{range .People}}<label>{{.Name}}<input name="rating_{{.ID}}" type="number" min="0" max="10" step="0.5" placeholder="0-10"></label>{{end}}</fieldset>
    <fieldset class="tags-field"><legend>Tags</legend>{{range .Tags}}<label><input type="checkbox" name="tag_id" value="{{.ID}}"> {{.Name}}</label>{{end}}<label>New tag<input name="new_tag" placeholder="Great fries"></label></fieldset>
    <label>Notes<textarea name="notes" rows="4" placeholder="What should we remember next time?"></textarea></label>
    <label class="inline-check"><input type="checkbox" name="is_chain" value="true"> Mark new restaurant as chain</label>
    <button class="primary-button">Save Dine</button>
  </form>
</main>
{{template "bottom" .}}
{{end}}

{{define "restaurant"}}
{{template "top" .}}
<main class="page-band">
  <section class="wide-ticket">
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
    <div class="record-grid">
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
<main class="page-band"><form class="login-card" method="post" action="/login"><h1>Kitchen Pass</h1><label>API token<input type="password" name="token" autofocus></label><input type="hidden" name="redirect" value="{{.Query}}"><button class="primary-button">Enter</button></form></main>
{{template "bottom" .}}
{{end}}

{{define "search"}}
{{template "top" .}}
<main class="page-band">
  <section class="wide-ticket">
    <h1>Have we eaten here before?</h1>
    <form class="search-row" method="get" action="/search"><input name="q" type="search" value="{{.Query}}" placeholder="Restaurant name"><button>Search</button></form>
    {{if .Restaurants}}<h2>Known Places</h2><div class="restaurant-list">{{range .Restaurants}}<a class="restaurant-row" href="/restaurants/{{.ID}}"><strong>{{.Name}}</strong><span>{{if .Address}}{{.Address}}{{end}}</span>{{if .IsChain}}<em>Chain</em>{{end}}</a>{{end}}</div>{{end}}
    {{if .Places}}<h2>Google Places</h2><div class="restaurant-list">{{range .Places}}<div class="restaurant-row"><strong>{{.DisplayName.Text}}</strong><span>{{.Address}}</span><span>{{.Rating}} · {{.PriceLevel}}</span></div>{{end}}</div>{{end}}
  </section>
</main>
{{template "bottom" .}}
{{end}}

{{define "nearby"}}
{{template "top" .}}
<main class="page-band">
  <section class="wide-ticket">
    <h1>Nearby</h1>
    <form class="search-row" method="get" action="/nearby"><input name="lat" placeholder="Latitude"><input name="lng" placeholder="Longitude"><button>Find Restaurants</button></form>
    {{if .Places}}<div class="restaurant-list">{{range .Places}}<div class="restaurant-row"><strong>{{.DisplayName.Text}}</strong><span>{{.Address}}</span><span>{{.Rating}} · {{.PriceLevel}}</span></div>{{end}}</div>{{else}}<p class="empty">Use your browser location or enter coordinates to search nearby restaurants.</p>{{end}}
  </section>
</main>
{{template "bottom" .}}
{{end}}
`
