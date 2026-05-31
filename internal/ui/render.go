package ui

import (
	"fmt"
	"html/template"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bitofbytes-io/dined/internal/apptime"
	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/places"
	"github.com/google/uuid"
)

type PageData struct {
	Title                   string
	Authenticated           bool
	Error                   string
	Notice                  string
	Visits                  []model.Visit
	ReadOnlyVisits          bool
	Visit                   *model.Visit
	People                  []model.Person
	Tags                    []model.Tag
	Restaurants             []model.Restaurant
	SearchResults           []RestaurantResult
	Restaurant              *model.Restaurant
	Stats                   model.Stats
	PickerTurn              model.PickerTurn
	TrophyMapPoints         []model.RestaurantMapPoint
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
	ReturnVisitID           string
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
  <script>
    (function () {
      if (window.location.hostname !== "dined.bitofbytes.io") {
        return;
      }

      var script = document.createElement("script");
      script.async = true;
      script.dataset.websiteId = "95e29ad0-5e97-4375-849b-fa5d7633d5a8";
      script.src = "https://dined.bitofbytes.io/umami/script.js";
      document.head.appendChild(script);
    })();
  </script>
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
    {{if .Notice}}<div class="flash notice" role="status">{{.Notice}}</div>{{end}}
{{end}}

{{define "bottom"}}
	  </div>
	  {{if .Authenticated}}{{template "delete-confirm-modal" .}}{{end}}
	  {{template "photo-preview-modal" .}}
	  <script>
	    var dinedPendingDeleteForm = null;
	    var dinedPreviewPhotos = [];
	    var dinedPreviewIndex = 0;
	    var dinedPhotoMaxBytes = {{maxVisitPhotoBytes}};

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
	      var city = form.querySelector("input[name='city']");
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
	      if (city) city.value = option.dataset.city || "";
	      if (place && !place.value) place.value = option.dataset.googlePlaceId || "";
	      if (category && !category.value) category.value = option.dataset.category || "";
	    }

		    function dinedValidateHalfStepRating(input) {
		      if (!input || input.value === "") {
		        input.setCustomValidity("");
		        return;
		      }
		      var rating = Number(input.value);
		      var isHalfStep = Number.isFinite(rating) && rating >= 0 && rating <= 10 && Number.isInteger(rating * 2);
		      input.setCustomValidity(isHalfStep ? "" : "Use whole numbers or .5 increments from 0 to 10.");
		    }

		    function dinedIsValidRatingValue(value) {
		      if (!value || !value.trim()) return false;
		      var rating = Number(value);
		      return Number.isFinite(rating) && rating >= 0 && rating <= 10 && Number.isInteger(rating * 2);
		    }

		    function dinedUpdateLogValidation(form) {
		      if (!form) return true;
		      var ratings = form.querySelectorAll("[data-half-step-rating]");
		      var hasRating = false;
		      for (var i = 0; i < ratings.length; i += 1) {
		        dinedValidateHalfStepRating(ratings[i]);
		        if (dinedIsValidRatingValue(ratings[i].value)) {
		          hasRating = true;
		        }
		      }

		      var message = form.querySelector("[data-rating-message]");
		      if (message) {
		        message.hidden = hasRating;
		      }
		      if (ratings.length && !hasRating && ratings[0].value === "") {
		        ratings[0].setCustomValidity("At least one rating is required.");
		      }

		      var restaurant = form.querySelector("input[name='restaurant_name']");
		      var visitedAt = form.querySelector("input[name='visited_at']");
		      var submit = form.querySelector("[data-log-submit]");
		      var hasRequiredFields = (!restaurant || restaurant.value.trim()) && (!visitedAt || visitedAt.value.trim());
		      var canSubmit = Boolean(hasRequiredFields && hasRating && form.checkValidity());
		      form.classList.toggle("log-form-incomplete", !canSubmit);
		      if (submit) {
		        submit.disabled = !canSubmit;
		        submit.setAttribute("aria-disabled", canSubmit ? "false" : "true");
		      }
		      return canSubmit;
		    }

		    function dinedInitializeLogValidation() {
		      var ratings = document.querySelectorAll("[data-half-step-rating]");
		      for (var i = 0; i < ratings.length; i += 1) {
		        dinedValidateHalfStepRating(ratings[i]);
		      }
		      var logForms = document.querySelectorAll("[data-log-form]");
		      for (var j = 0; j < logForms.length; j += 1) {
		        dinedUpdateLogValidation(logForms[j]);
		      }
		    }

	    function dinedPhotoLimit(uploader) {
	      var limit = uploader ? Number(uploader.dataset.photoLimit) : 0;
	      return Number.isFinite(limit) && limit > 0 ? limit : 4;
	    }

	    function dinedPhotoCount(uploader) {
	      return uploader ? uploader.querySelectorAll("[data-photo-item]").length : 0;
	    }

	    function dinedSetPhotoError(uploader, message) {
	      var error = uploader ? uploader.querySelector("[data-photo-error]") : null;
	      if (!error) return;
	      error.textContent = message || "";
	      error.hidden = !message;
	    }

	    function dinedUpdatePhotoUploader(uploader) {
	      if (!uploader) return;
	      var addTile = uploader.querySelector("[data-photo-add]");
	      var input = uploader.querySelector("[data-photo-input]");
	      var count = dinedPhotoCount(uploader);
	      var atLimit = count >= dinedPhotoLimit(uploader);
	      if (addTile) addTile.hidden = atLimit;
	      if (input) input.disabled = atLimit;
	    }

	    function dinedInitializePhotoUploaders() {
	      var uploaders = document.querySelectorAll("[data-photo-uploader]");
	      for (var i = 0; i < uploaders.length; i += 1) {
	        dinedUpdatePhotoUploader(uploaders[i]);
	      }
	    }

	    function dinedDataURIByteCount(dataURI) {
	      var encoded = (dataURI.split(",", 2)[1] || "").trim();
	      var padding = 0;
	      if (encoded.endsWith("==")) padding = 2;
	      else if (encoded.endsWith("=")) padding = 1;
	      return Math.floor((encoded.length * 3) / 4) - padding;
	    }

	    function dinedDrawPhotoDataURI(image, maxDimension, quality) {
	      var width = image.naturalWidth || image.width;
	      var height = image.naturalHeight || image.height;
	      var scale = Math.min(1, maxDimension / Math.max(width, height));
	      var canvas = document.createElement("canvas");
	      canvas.width = Math.max(1, Math.round(width * scale));
	      canvas.height = Math.max(1, Math.round(height * scale));
	      var ctx = canvas.getContext("2d");
	      ctx.fillStyle = "#fffdf4";
	      ctx.fillRect(0, 0, canvas.width, canvas.height);
	      ctx.drawImage(image, 0, 0, canvas.width, canvas.height);
	      return canvas.toDataURL("image/jpeg", quality);
	    }

	    function dinedCompressPhoto(file) {
	      return new Promise(function (resolve, reject) {
	        if (!file || (file.type && file.type.indexOf("image/") !== 0)) {
	          reject(new Error("Only image files can be added."));
	          return;
	        }
	        var image = new Image();
	        var url = URL.createObjectURL(file);
	        image.onload = function () {
	          URL.revokeObjectURL(url);
	          var attempts = [
	            { dimension: 1280, quality: .72 },
	            { dimension: 1120, quality: .68 },
	            { dimension: 960, quality: .64 },
	            { dimension: 800, quality: .6 }
	          ];
	          for (var i = 0; i < attempts.length; i += 1) {
	            var dataURI = dinedDrawPhotoDataURI(image, attempts[i].dimension, attempts[i].quality);
	            if (dinedDataURIByteCount(dataURI) <= dinedPhotoMaxBytes) {
	              resolve(dataURI);
	              return;
	            }
	          }
	          reject(new Error("That photo is too large to save here. Try a smaller image."));
	        };
	        image.onerror = function () {
	          URL.revokeObjectURL(url);
	          reject(new Error("That photo could not be read."));
	        };
	        image.src = url;
	      });
	    }

	    function dinedAddPhotoTile(uploader, dataURI) {
	      var list = uploader.querySelector("[data-photo-upload-list]");
	      var addTile = uploader.querySelector("[data-photo-add]");
	      if (!list || !addTile) return;

	      var tile = document.createElement("div");
	      tile.className = "photo-upload-tile";
	      tile.setAttribute("data-photo-item", "");

	      var preview = document.createElement("button");
	      preview.type = "button";
	      preview.className = "photo-thumb photo-upload-preview";
	      preview.setAttribute("data-photo-preview", "");
	      preview.setAttribute("data-photo-src", dataURI);
	      preview.setAttribute("data-photo-alt", "New dine photo");

	      var img = document.createElement("img");
	      img.src = dataURI;
	      img.alt = "";
	      preview.appendChild(img);

	      var hidden = document.createElement("input");
	      hidden.type = "hidden";
	      hidden.name = "photo_data_uri";
	      hidden.value = dataURI;

	      var remove = document.createElement("button");
	      remove.type = "button";
	      remove.className = "photo-remove-button";
	      remove.setAttribute("data-photo-remove", "");
	      remove.setAttribute("aria-label", "Remove photo");
	      remove.textContent = "Remove";

	      tile.appendChild(preview);
	      tile.appendChild(hidden);
	      tile.appendChild(remove);
	      list.insertBefore(tile, addTile);
	      dinedUpdatePhotoUploader(uploader);
	    }

	    function dinedHandlePhotoInput(input) {
	      var uploader = input.closest("[data-photo-uploader]");
	      if (!uploader) return;
	      dinedSetPhotoError(uploader, "");
	      var files = Array.prototype.slice.call(input.files || []);
	      var slots = dinedPhotoLimit(uploader) - dinedPhotoCount(uploader);
	      if (slots <= 0) {
	        dinedSetPhotoError(uploader, "Remove a photo before adding another.");
	        input.value = "";
	        return;
	      }
	      if (files.length > slots) {
	        dinedSetPhotoError(uploader, "Only " + dinedPhotoLimit(uploader) + " photos can be saved for one dine.");
	        files = files.slice(0, slots);
	      }
	      if (!files.length) return;

	      var index = 0;
	      function next() {
	        if (index >= files.length) {
	          input.value = "";
	          dinedUpdatePhotoUploader(uploader);
	          return;
	        }
	        dinedCompressPhoto(files[index]).then(function (dataURI) {
	          dinedAddPhotoTile(uploader, dataURI);
	          index += 1;
	          next();
	        }).catch(function (error) {
	          dinedSetPhotoError(uploader, error.message || "Photo could not be added.");
	          index += 1;
	          next();
	        });
	      }
	      next();
	    }

	    function dinedRemovePhoto(button) {
	      var tile = button.closest("[data-photo-item]");
	      var uploader = button.closest("[data-photo-uploader]");
	      if (tile) tile.remove();
	      dinedSetPhotoError(uploader, "");
	      dinedUpdatePhotoUploader(uploader);
	    }

	    function dinedOpenPhotoPreview(button) {
	      var source = button.dataset.photoSrc;
	      if (!source) return;
	      var scope = button.closest("[data-photo-strip], [data-photo-uploader]");
	      var buttons = scope ? Array.prototype.slice.call(scope.querySelectorAll("[data-photo-preview]")) : [button];
	      dinedPreviewPhotos = buttons.map(function (item) {
	        return { src: item.dataset.photoSrc || "", alt: item.dataset.photoAlt || "Dine photo" };
	      }).filter(function (item) { return item.src; });
	      dinedPreviewIndex = Math.max(0, buttons.indexOf(button));
	      if (!dinedPreviewPhotos.length) return;
	      var modal = document.getElementById("photo-preview-modal");
	      if (!modal || typeof modal.showModal !== "function") {
	        window.open(source, "_blank", "noopener");
	        return;
	      }
	      dinedRenderPhotoPreview();
	      if (!modal.open) modal.showModal();
	      if (typeof modal.focus === "function") {
	        modal.focus({ preventScroll: true });
	      }
	    }

	    function dinedRenderPhotoPreview() {
	      if (!dinedPreviewPhotos.length) return;
	      if (dinedPreviewIndex < 0) dinedPreviewIndex = dinedPreviewPhotos.length - 1;
	      if (dinedPreviewIndex >= dinedPreviewPhotos.length) dinedPreviewIndex = 0;
	      var photo = dinedPreviewPhotos[dinedPreviewIndex];
	      var image = document.getElementById("photo-preview-image");
	      var count = document.getElementById("photo-preview-count");
	      var prev = document.getElementById("photo-preview-prev");
	      var next = document.getElementById("photo-preview-next");
	      if (image) {
	        image.src = photo.src;
	        image.alt = photo.alt;
	      }
	      if (count) count.textContent = (dinedPreviewIndex + 1) + " / " + dinedPreviewPhotos.length;
	      if (prev) prev.disabled = dinedPreviewPhotos.length < 2;
	      if (next) next.disabled = dinedPreviewPhotos.length < 2;
	    }

	    function dinedShiftPhotoPreview(delta) {
	      if (!dinedPreviewPhotos.length) return;
	      dinedPreviewIndex += delta;
	      dinedRenderPhotoPreview();
	    }

	    function dinedClosePhotoPreview() {
	      var modal = document.getElementById("photo-preview-modal");
	      if (modal && modal.open) modal.close();
	    }

	    function dinedHandleInput(event) {
	      if (event.target.name === "restaurant_name" || event.target.name === "address") {
	        dinedSyncRestaurantSelection(event.target.form);
	      }
	      if (event.target.matches("[data-half-step-rating]")) {
	        dinedValidateHalfStepRating(event.target);
	      }
	      var logForm = event.target.closest ? event.target.closest("[data-log-form]") : null;
	      if (logForm) {
	        dinedUpdateLogValidation(logForm);
	      }
	    }

	    function dinedHandleChange(event) {
	      if (event.target.matches("[data-photo-input]")) {
	        dinedHandlePhotoInput(event.target);
	      }
	      var logForm = event.target.closest ? event.target.closest("[data-log-form]") : null;
	      if (logForm) {
	        dinedUpdateLogValidation(logForm);
	      }
	    }

	    function dinedHandleClick(event) {
	      if (event.target && event.target.id === "photo-preview-modal") {
	        dinedClosePhotoPreview();
	        return;
	      }
	      var previewButton = event.target.closest ? event.target.closest("[data-photo-preview]") : null;
	      if (previewButton) {
	        event.preventDefault();
	        dinedOpenPhotoPreview(previewButton);
	        return;
	      }
	      var removeButton = event.target.closest ? event.target.closest("[data-photo-remove]") : null;
	      if (removeButton) {
	        event.preventDefault();
	        dinedRemovePhoto(removeButton);
	        return;
	      }
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
	    }

	    function dinedHandleSubmit(event) {
	      var form = event.target;
	      if (form && form.matches("[data-log-form]")) {
	        dinedSyncRestaurantSelection(form);
	        if (!dinedUpdateLogValidation(form)) {
	          event.preventDefault();
	          if (typeof form.reportValidity === "function") {
	            form.reportValidity();
	          }
	        }
	        return;
	      }
	      if (!form || form.id !== "nearby-form") return;
	      var lat = form.querySelector("input[name='lat']");
	      var lng = form.querySelector("input[name='lng']");
	      var near = form.querySelector("input[name='near']");
	      if (!lat || !lng || (lat.value.trim() && lng.value.trim()) || (near && near.value.trim())) return;
	      event.preventDefault();
	      var useLocation = document.getElementById("use-location");
	      if (useLocation) useLocation.click();
	    }

	    function dinedConfirmDelete(event, form) {
	      if (event) event.preventDefault();
	      if (!form) return false;
	      var modal = document.getElementById("delete-dine-modal");
	      var title = form.dataset.deleteTitle || "Delete this item?";
	      var message = form.dataset.deleteMessage || "This action cannot be undone.";
	      var confirmText = form.dataset.deleteConfirm || "Delete";
	      if (!modal || typeof modal.showModal !== "function") {
	        if (confirm(title)) dinedSubmitDeleteForm(form);
	        return false;
	      }
	      dinedPendingDeleteForm = form;
	      var titleNode = document.getElementById("delete-dine-title");
	      var messageNode = document.getElementById("delete-dine-message");
	      var confirmButton = document.getElementById("delete-dine-confirm-button");
	      if (titleNode) titleNode.textContent = title;
	      if (messageNode) messageNode.textContent = message;
	      if (confirmButton) confirmButton.textContent = confirmText;
	      if (!modal.open) modal.showModal();
	      return false;
	    }

	    function dinedSubmitPendingDelete() {
	      var form = dinedPendingDeleteForm;
	      dinedPendingDeleteForm = null;
	      if (!form) return;
	      var modal = document.getElementById("delete-dine-modal");
	      if (modal && modal.open) modal.close();
	      dinedSubmitDeleteForm(form);
	    }

	    function dinedCancelDelete(button) {
	      dinedPendingDeleteForm = null;
	      var modal = button ? button.closest("dialog") : document.getElementById("delete-dine-modal");
	      if (modal && modal.open) modal.close();
	    }

	    function dinedSubmitDeleteForm(form) {
	      HTMLFormElement.prototype.submit.call(form);
	    }

	    (function () {
	      var modal = document.getElementById("delete-dine-modal");
	      if (!modal) return;
	      modal.addEventListener("close", function () {
	        dinedPendingDeleteForm = null;
	      });
	    }());

	    function dinedHandleKeydown(event) {
	      var modal = document.getElementById("photo-preview-modal");
	      if (!modal || !modal.open) return;
	      if (event.key === "ArrowLeft") dinedShiftPhotoPreview(-1);
	      if (event.key === "ArrowRight") dinedShiftPhotoPreview(1);
	    }

	    function dinedInitializePage() {
	      dinedInitializeLogValidation();
	      dinedInitializePhotoUploaders();
	    }

	    if (!window.dinedListenersBound) {
	      window.dinedListenersBound = true;
	      document.addEventListener("input", dinedHandleInput);
	      document.addEventListener("change", dinedHandleChange);
	      document.addEventListener("click", dinedHandleClick);
	      document.addEventListener("submit", dinedHandleSubmit);
	      document.addEventListener("keydown", dinedHandleKeydown);
	      document.addEventListener("htmx:load", dinedInitializePage);
	    }
	    dinedInitializePage();
	  </script>
	</body>
	</html>
{{end}}

{{define "delete-confirm-modal"}}
  <dialog class="confirm-modal" id="delete-dine-modal" aria-labelledby="delete-dine-title" aria-describedby="delete-dine-message">
    <div class="confirm-card">
      <h2 id="delete-dine-title">Delete this item?</h2>
      <p id="delete-dine-message">This action cannot be undone.</p>
      <div class="confirm-actions">
        <button type="button" class="secondary-button" id="delete-dine-cancel-button" onclick="dinedCancelDelete(this)">Cancel</button>
        <button type="button" class="danger" id="delete-dine-confirm-button" onclick="dinedSubmitPendingDelete()">Delete</button>
      </div>
    </div>
  </dialog>
{{end}}

{{define "photo-preview-modal"}}
  <dialog class="photo-preview-modal" id="photo-preview-modal" aria-labelledby="photo-preview-title" tabindex="-1">
    <div class="photo-preview-card">
      <div class="photo-preview-head">
        <h2 id="photo-preview-title">Dine Photo</h2>
        <button type="button" class="secondary-button photo-preview-close" onclick="dinedClosePhotoPreview()">Close</button>
      </div>
      <div class="photo-preview-stage">
        <img id="photo-preview-image" alt="Dine photo">
      </div>
      <div class="photo-preview-actions">
        <button type="button" class="secondary-button" id="photo-preview-prev" onclick="dinedShiftPhotoPreview(-1)">Previous</button>
        <span id="photo-preview-count">1 / 1</span>
        <button type="button" class="secondary-button" id="photo-preview-next" onclick="dinedShiftPhotoPreview(1)">Next</button>
      </div>
    </div>
  </dialog>
{{end}}

{{define "visit-list"}}
{{if .Visits}}
  <div class="visit-list">
  {{range .Visits}}
    {{$visit := .}}<article class="visit-card" id="{{.ID}}">
      <div class="visit-main">
        <h3><a href="/restaurants/{{.Restaurant.ID}}">{{.Restaurant.Name}}</a>{{if .Restaurant.IsChain}} <span class="badge">Chain</span>{{end}}</h3>
        <p class="visit-meta">{{date .VisitedAt}} · Picked by {{.Picker.Name}} · {{dollars .PriceLevel}}</p>
        {{if .Restaurant.Address}}<p class="muted">{{.Restaurant.Address}}</p>{{end}}
      </div>
      <div class="score-disc">{{avg .}}</div>
      <div class="ratings">{{range .Ratings}}<span>{{.Person.Name}} {{score .Score}}</span>{{end}}</div>
      {{if .Tags}}<div class="tags">{{range .Tags}}<span>{{.Name}}</span>{{end}}</div>{{end}}
      {{if .Notes}}<p class="note">{{.Notes}}</p>{{end}}
      {{if .Photos}}<div class="visit-photo-strip" data-photo-strip>{{range $index, $photo := .Photos}}<button type="button" class="photo-thumb" data-photo-preview data-photo-src="{{imageSrc $photo.DataURI}}" data-photo-alt="{{$visit.Restaurant.Name}} photo {{add1 $index}}"><img src="{{imageSrc $photo.DataURI}}" alt=""></button>{{end}}</div>{{end}}
      {{if and $.Authenticated (not $.ReadOnlyVisits)}}<div class="visit-actions"><a class="secondary-button" href="/visits/{{.ID}}/edit">Edit</a><form method="post" action="/visits/{{.ID}}/delete" hx-boost="false" data-delete-dine-form data-delete-title="Delete this dine?" data-delete-message="This removes the dine and its ratings from the ledger." data-delete-confirm="Delete" onsubmit="return dinedConfirmDelete(event, this)"><button class="danger">Delete</button></form></div>{{end}}
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
      <div class="section-head"><div><h2>Recent Dines</h2>{{if .PickerTurn.NextPicker.Name}}<p class="next-up">Next Up: {{.PickerTurn.NextPicker.Name}}</p>{{end}}</div><a href="/dines">View all</a></div>
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
		<form class="wide-ticket log-form order-pad console-pad" method="post" action="/visits" data-log-form>
	    <div class="section-head console-head">
		      <div><h1>Log a Dine</h1></div>
	    </div>
    {{if .PrefillPlaceID}}<p class="place-source">Google place loaded: {{.PrefillName}}</p>{{end}}
    <div class="console-grid">
	      <section class="form-section restaurant-console">
	        <input type="hidden" name="restaurant_id" value="{{.PrefillRestaurantID}}">
	        <input type="hidden" name="city" value="{{.PrefillCity}}">
	        <input type="hidden" name="latitude" value="{{.PrefillLatitude}}">
	        <input type="hidden" name="longitude" value="{{.PrefillLongitude}}">
	        <input type="hidden" name="phone" value="{{.PrefillPhone}}">
	        <input type="hidden" name="website" value="{{.PrefillWebsite}}">
	        <input type="hidden" name="google_rating" value="{{.PrefillGoogleRating}}">
	        <input type="hidden" name="google_price_level" value="{{.PrefillGooglePriceLevel}}">
	        <div class="form-grid restaurant-grid">
		          <label>Restaurant<input name="restaurant_name" list="restaurant-options" placeholder="Search or add restaurant" value="{{.PrefillName}}" required><datalist id="restaurant-options">{{range .Restaurants}}<option value="{{.Name}}" data-restaurant-id="{{.ID}}" data-address="{{if .Address}}{{.Address}}{{end}}" data-city="{{if .City}}{{.City}}{{end}}" data-google-place-id="{{if .GooglePlaceID}}{{.GooglePlaceID}}{{end}}" data-category="{{if .Category}}{{.Category}}{{end}}">{{if .Address}}{{.Address}}{{end}}</option>{{end}}</datalist></label>
	          <label>Address<input name="address" placeholder="Optional" value="{{.PrefillAddress}}"></label>
          <label>Google Place ID<input name="google_place_id" placeholder="Optional" value="{{.PrefillPlaceID}}"></label>
          <label>Category<select name="category"><option></option><option {{if eq .PrefillCategory "American"}}selected{{end}}>American</option><option {{if eq .PrefillCategory "Mexican"}}selected{{end}}>Mexican</option><option {{if eq .PrefillCategory "Italian"}}selected{{end}}>Italian</option><option {{if eq .PrefillCategory "Pizza"}}selected{{end}}>Pizza</option><option {{if eq .PrefillCategory "Burgers"}}selected{{end}}>Burgers</option><option {{if eq .PrefillCategory "Breakfast"}}selected{{end}}>Breakfast</option><option {{if eq .PrefillCategory "Chinese"}}selected{{end}}>Chinese</option><option {{if eq .PrefillCategory "Japanese"}}selected{{end}}>Japanese</option><option {{if eq .PrefillCategory "Korean"}}selected{{end}}>Korean</option><option {{if eq .PrefillCategory "Thai"}}selected{{end}}>Thai</option><option {{if eq .PrefillCategory "Indian"}}selected{{end}}>Indian</option><option {{if eq .PrefillCategory "BBQ"}}selected{{end}}>BBQ</option><option {{if eq .PrefillCategory "Seafood"}}selected{{end}}>Seafood</option><option {{if eq .PrefillCategory "Dessert"}}selected{{end}}>Dessert</option><option {{if eq .PrefillCategory "Coffee"}}selected{{end}}>Coffee</option><option {{if eq .PrefillCategory "Other"}}selected{{end}}>Other</option></select></label>
        </div>
      </section>
      <section class="form-section visit-console">
        <div class="form-grid visit-form-grid">
	          <label>Date and time<input type="datetime-local" name="visited_at" value="{{.NowLocal}}" required></label>
          <label>Picked by<select name="picker_id" required>{{range .People}}<option value="{{.ID}}" {{if eq $.PrefillPickerID .ID.String}}selected{{end}}>{{.Name}}</option>{{end}}</select></label>
          <label>Price Level<select name="price_level"><option value="1" {{if eq .PrefillPriceLevel 1}}selected{{end}}>$</option><option value="2" {{if or (eq .PrefillPriceLevel 0) (eq .PrefillPriceLevel 2)}}selected{{end}}>$$</option><option value="3" {{if eq .PrefillPriceLevel 3}}selected{{end}}>$$$</option><option value="4" {{if eq .PrefillPriceLevel 4}}selected{{end}}>$$$$</option></select></label>
        </div>
      </section>
    </div>
		    <fieldset class="ratings-field score-console" aria-describedby="rating-requirement"><legend>Rate Your Experience</legend><p class="rating-requirement" id="rating-requirement" data-rating-message aria-live="polite"{{if prefillHasRating .PrefillRatings}} hidden{{end}}>At least one rating is required before saving.</p>{{range .People}}<label class="rating-card">{{if avatar .Name}}<img class="avatar-face" src="{{avatar .Name}}" alt="">{{else}}<span class="avatar-dot">{{slice .Name 0 1}}</span>{{end}}<span>{{.Name}}</span><input name="rating_{{.ID}}" type="number" min="0" max="10" step="0.5" inputmode="decimal" placeholder="0-10" value="{{prefillRatingValue $.PrefillRatings .ID}}" data-half-step-rating></label>{{end}}</fieldset>
		    <fieldset class="tags-field chip-field"><legend>Tags</legend>{{range .Tags}}<label><input type="checkbox" name="tag_id" value="{{.ID}}" {{if prefillTagChecked $.PrefillTagIDs .ID}}checked{{end}}> <span>{{.Name}}</span></label>{{end}}<label class="new-tag">New tag<input name="new_tag" placeholder="Great fries" value="{{.PrefillNewTag}}"></label></fieldset>
	    <label class="notes-field">Notes<textarea name="notes" rows="4" placeholder="What should we remember next time?">{{.PrefillNotes}}</textarea></label>
	    <section class="form-section photo-console" data-photo-uploader data-photo-limit="{{maxVisitPhotos}}">
	      <div class="photo-console-head"><div><h2>Photos</h2><p>Food, fun, memories. Up to {{maxVisitPhotos}}.</p></div></div>
	      <div class="photo-upload-strip" data-photo-upload-list data-photo-strip>
	        {{range .PrefillPhotoDataURIs}}<div class="photo-upload-tile" data-photo-item><button type="button" class="photo-thumb photo-upload-preview" data-photo-preview data-photo-src="{{imageSrc .}}" data-photo-alt="New dine photo"><img src="{{imageSrc .}}" alt=""></button><input type="hidden" name="photo_data_uri" value="{{.}}"><button type="button" class="photo-remove-button" data-photo-remove aria-label="Remove photo">Remove</button></div>{{end}}
	        <label class="photo-add-tile" data-photo-add><span class="photo-add-mark">+</span><span>Add Photos</span><input type="file" accept="image/*" multiple data-photo-input hidden></label>
	      </div>
	      <p class="photo-error" data-photo-error hidden></p>
	    </section>
	    <div class="form-actions console-actions"><label class="inline-check"><input type="checkbox" name="is_chain" value="true" {{if .PrefillIsChain}}checked{{end}}> Mark new restaurant as chain</label><button class="primary-button" data-log-submit>Save Dine</button></div>
  </form>
</main>
{{template "bottom" .}}
{{end}}

{{define "visit-edit"}}
{{template "top" .}}
<main class="page-band order-page">
  {{with .Visit}}{{$visit := .}}
  <form class="wide-ticket log-form order-pad console-pad" method="post" action="/visits/{{.ID}}">
    <div class="section-head console-head">
      <div><h1>Edit Dine</h1><p>{{.Restaurant.Name}}{{if .Restaurant.Address}} · {{.Restaurant.Address}}{{end}}</p></div>
      <a class="secondary-button" href="/dines#{{.ID}}">Cancel</a>
    </div>
    <input type="hidden" name="restaurant_id" value="{{.Restaurant.ID}}">
    <section class="form-section visit-console">
      <div class="form-grid visit-form-grid">
        <label>Date and time<input type="datetime-local" name="visited_at" value="{{datetime .VisitedAt}}"></label>
        <label>Picked by<select name="picker_id" required>{{range $.People}}<option value="{{.ID}}" {{if eq $visit.Picker.ID .ID}}selected{{end}}>{{.Name}}</option>{{end}}</select></label>
        <label>Price Level<select name="price_level"><option value="1" {{if eq .PriceLevel 1}}selected{{end}}>$</option><option value="2" {{if eq .PriceLevel 2}}selected{{end}}>$$</option><option value="3" {{if eq .PriceLevel 3}}selected{{end}}>$$$</option><option value="4" {{if eq .PriceLevel 4}}selected{{end}}>$$$$</option></select></label>
      </div>
    </section>
    <fieldset class="ratings-field score-console"><legend>Rate Your Experience</legend>{{range $.People}}<label class="rating-card">{{if avatar .Name}}<img class="avatar-face" src="{{avatar .Name}}" alt="">{{else}}<span class="avatar-dot">{{slice .Name 0 1}}</span>{{end}}<span>{{.Name}}</span><input name="rating_{{.ID}}" type="number" min="0" max="10" step="0.5" inputmode="decimal" placeholder="0-10" value="{{ratingValue $visit .}}" data-half-step-rating></label>{{end}}</fieldset>
    <fieldset class="tags-field chip-field"><legend>Tags</legend>{{range $.Tags}}<label><input type="checkbox" name="tag_id" value="{{.ID}}" {{if hasVisitTag $visit .}}checked{{end}}> <span>{{.Name}}</span></label>{{end}}<label class="new-tag">New tag<input name="new_tag" placeholder="Great fries"></label></fieldset>
    <label class="notes-field">Notes<textarea name="notes" rows="4" placeholder="What should we remember next time?">{{str .Notes}}</textarea></label>
    <section class="form-section photo-console" data-photo-uploader data-photo-limit="{{maxVisitPhotos}}">
      <div class="photo-console-head"><div><h2>Photos</h2><p>Food, fun, memories. Up to {{maxVisitPhotos}}.</p></div></div>
      <div class="photo-upload-strip" data-photo-upload-list data-photo-strip>
        {{range $index, $photo := .Photos}}<div class="photo-upload-tile" data-photo-item><button type="button" class="photo-thumb photo-upload-preview" data-photo-preview data-photo-src="{{imageSrc $photo.DataURI}}" data-photo-alt="{{$visit.Restaurant.Name}} photo {{add1 $index}}"><img src="{{imageSrc $photo.DataURI}}" alt=""></button><input type="hidden" name="keep_photo_id" value="{{$photo.ID}}"><button type="button" class="photo-remove-button" data-photo-remove aria-label="Remove photo">Remove</button></div>{{end}}
        <label class="photo-add-tile" data-photo-add><span class="photo-add-mark">+</span><span>Add Photos</span><input type="file" accept="image/*" multiple data-photo-input hidden></label>
      </div>
      <p class="photo-error" data-photo-error hidden></p>
    </section>
    <div class="form-actions console-actions"><a class="secondary-button" href="/restaurants/{{.Restaurant.ID}}/edit?return_visit_id={{.ID}}">Edit Restaurant Details</a><button class="primary-button">Save Changes</button></div>
  </form>
  {{end}}
</main>
{{template "bottom" .}}
{{end}}

{{define "restaurant"}}
{{template "top" .}}
<main class="page-band ledger-page">
  <section class="wide-ticket ledger-panel">
    {{with .Restaurant}}
    <div class="section-head"><div><h1>{{.Name}} {{if .IsChain}}<span class="badge">Chain</span>{{end}}</h1>{{if .Address}}<p>{{.Address}}</p>{{end}}</div></div>
    <dl class="details"><div class="detail-item"><dt>Category</dt><dd>{{if .Category}}{{.Category}}{{else}}-{{end}}</dd></div><div class="detail-item"><dt>Address</dt><dd>{{if .Address}}{{.Address}}{{else}}-{{end}}</dd></div><div class="detail-item"><dt>City</dt><dd>{{if .City}}{{.City}}{{else}}-{{end}}</dd></div><div class="detail-item"><dt>Phone</dt><dd>{{if .Phone}}{{.Phone}}{{else}}-{{end}}</dd></div><div class="detail-item"><dt>Website</dt><dd>{{if .Website}}<a href="{{.Website}}">{{.Website}}</a>{{else}}-{{end}}</dd></div><div class="detail-item"><dt>Google rating</dt><dd>{{if .GoogleRating}}{{floatInput .GoogleRating}}{{else}}-{{end}}</dd></div><div class="detail-item"><dt>Google price</dt><dd>{{if .GooglePriceLevel}}{{dollars (intValue .GooglePriceLevel)}}{{else}}-{{end}}</dd></div></dl>
    {{end}}
    {{template "visit-list" .}}
  </section>
</main>
{{template "bottom" .}}
{{end}}

{{define "restaurant-edit"}}
{{template "top" .}}
<main class="page-band order-page">
  {{with .Restaurant}}
  {{if .GooglePlaceID}}<form id="restaurant-google-refresh-form" method="post" action="/restaurants/{{.ID}}/google-refresh">{{with $.ReturnVisitID}}<input type="hidden" name="return_visit_id" value="{{.}}">{{end}}</form>{{end}}
  <form class="wide-ticket log-form order-pad console-pad" method="post" action="/restaurants/{{.ID}}">
    <div class="section-head console-head">
      <div><h1>Edit Restaurant</h1>{{if .GooglePlaceID}}<p>Google place: {{str .GooglePlaceID}}</p>{{end}}</div>
      <a class="secondary-button" href="/restaurants/{{.ID}}">Cancel</a>
    </div>
    {{with $.ReturnVisitID}}<input type="hidden" name="return_visit_id" value="{{.}}">{{end}}
    <section class="form-section restaurant-console">
      <div class="form-grid restaurant-edit-grid">
        <label>Restaurant<input name="restaurant_name" value="{{.Name}}" required></label>
        <label>Category<input name="category" placeholder="American, Southern, Sushi..." value="{{str .Category}}"></label>
        <label>Address<input name="address" placeholder="Optional" value="{{str .Address}}"></label>
        <label>City<input name="city" placeholder="Optional" value="{{str .City}}"></label>
        <label>Phone<input name="phone" placeholder="Optional" value="{{str .Phone}}"></label>
        <label>Website<input name="website" type="url" placeholder="https://example.com" value="{{str .Website}}"></label>
        <label>Google Rating<input name="google_rating" type="number" min="0" max="5" step="0.1" inputmode="decimal" placeholder="0-5" value="{{floatInput .GoogleRating}}"></label>
        <label>Google Price<select name="google_price_level"><option></option><option value="1" {{if eq (intValue .GooglePriceLevel) 1}}selected{{end}}>$</option><option value="2" {{if eq (intValue .GooglePriceLevel) 2}}selected{{end}}>$$</option><option value="3" {{if eq (intValue .GooglePriceLevel) 3}}selected{{end}}>$$$</option><option value="4" {{if eq (intValue .GooglePriceLevel) 4}}selected{{end}}>$$$$</option></select></label>
      </div>
    </section>
    <div class="form-actions console-actions">
      <div>{{with $.ReturnVisitID}}<a class="secondary-button" href="/visits/{{.}}/edit">Return to Edit Dine</a>{{end}}</div>
      <div class="section-actions"><label class="inline-check"><input type="checkbox" name="is_chain" value="true" {{if .IsChain}}checked{{end}}> Mark as chain</label>{{if .GooglePlaceID}}<button class="secondary-button" type="submit" form="restaurant-google-refresh-form">Refresh Google Info</button>{{end}}<button class="primary-button">Save Restaurant</button></div>
    </div>
  </form>
  {{end}}
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
      <div class="record"><span class="record-value">{{.Stats.TotalDines}}</span><p>Total Dines</p></div>
      <div class="record"><span class="record-value">{{score .Stats.AverageRating}}</span><p>Family Average</p></div>
      <div class="record"><span class="record-value record-name">{{default .Stats.BestPicker "Waiting"}}</span><p>Best Picker</p>{{if .Stats.BestPicker}}<small>{{score .Stats.BestPickerAverage}} average</small>{{else}}<small>More dines needed</small>{{end}}</div>
      <div class="record"><span class="record-value record-name">{{default .Stats.WorstPicker "Waiting"}}</span><p>Worst Picker</p>{{if .Stats.WorstPicker}}<small>{{score .Stats.WorstPickerAverage}} average</small>{{else}}<small>More dines needed</small>{{end}}</div>
      <div class="record"><span class="record-value record-name">{{default .PickerTurn.NextPicker.Name "Waiting"}}</span><p>Next Up</p><small>Picker turn</small></div>
      <div class="record"><span class="record-value">{{.Stats.NewPlaces}}</span><p>New Places</p></div>
      <div class="record"><span class="record-value">{{.Stats.CitiesExplored}}</span><p>Cities Explored</p></div>
    </div>
    <div class="dined-map-panel">
      <div class="dined-map-heading"><p>Places We've Dined</p>{{if .TrophyMapPoints}}<span>{{len .TrophyMapPoints}} pinned</span>{{end}}</div>
      {{if .TrophyMapReady}}<img src="/trophy-case/map.png" alt="Map of places where the family has dined" width="640" height="360" loading="lazy">{{else}}<div class="dined-map-empty">{{default .TrophyMapFallback "No mapped dines yet"}}</div>{{end}}
    </div>
    <div class="track-list">
      <h2>All-Time Top Restaurants</h2>
      {{if .Stats.TopRestaurants}}<ol class="top-restaurant-list">{{range $index, $restaurant := .Stats.TopRestaurants}}<li><span class="track-rank">{{add1 $index}}</span><span class="track-name">{{$restaurant.Name}}</span><span class="track-score">{{score $restaurant.AverageRating}} avg</span></li>{{end}}</ol>{{else}}<p class="track-empty">Waiting on more dines</p>{{end}}
      <h2 class="track-section-title">Best By Cuisine</h2>
      {{if .Stats.TopRestaurantsByCuisine}}<ol class="top-restaurant-list cuisine-restaurant-list">{{range $restaurant := .Stats.TopRestaurantsByCuisine}}<li><span class="track-cuisine">{{$restaurant.Cuisine}}</span><span class="track-name">{{$restaurant.Name}}</span><span class="track-score">{{score $restaurant.AverageRating}} avg</span></li>{{end}}</ol>{{else}}<p class="track-empty">Waiting on cuisine data</p>{{end}}
    </div>
  </section>
</main>
{{template "bottom" .}}
{{end}}

{{define "login"}}
{{template "top" .}}
<main class="page-band pass-page"><section class="login-card kitchen-pass"><p class="eyebrow">Private Counter</p><h1>Kitchen Pass</h1><a class="primary-button" href="/api/auth/google{{if .Query}}?redirect={{query .Query}}{{end}}" hx-boost="false">Continue with Google</a></section></main>
{{template "bottom" .}}
{{end}}

{{define "search"}}
{{template "top" .}}
<main class="page-band search-page">
  <section class="wide-ticket sign-panel search-panel">
    <p class="eyebrow">Counter Search</p>
    <h1>Have we eaten here before?</h1>
    <form class="search-row" method="get" action="/search"><input name="q" type="search" value="{{.Query}}" placeholder="Restaurant name"><button>Search</button></form>
    {{if .SearchResults}}<h2>Saved Spots</h2><div class="restaurant-list history-list">{{range .SearchResults}}<article class="restaurant-row history-row"><div class="history-title"><a href="/restaurants/{{.Restaurant.ID}}"><strong>{{.Restaurant.Name}}</strong></a>{{if .Restaurant.IsChain}}<em>Chain</em>{{end}}</div><span>{{if .Restaurant.Address}}{{.Restaurant.Address}}{{end}}</span>{{with .LatestVisit}}<span>Last visit {{date .VisitedAt}} · Picked by {{.Picker.Name}} · {{dollars .PriceLevel}}</span>{{end}}<div class="row-stats"><span>{{.VisitCount}} {{if eq .VisitCount 1}}visit{{else}}visits{{end}}</span><span>Avg {{score .AverageRating}}</span></div>{{if .Tags}}<div class="tags">{{range .Tags}}<span>{{.Name}}</span>{{end}}</div>{{end}}{{if and $.Authenticated (eq .VisitCount 0)}}<form class="history-remove-form" method="post" action="/restaurants/{{.Restaurant.ID}}/delete" hx-boost="false" data-delete-title="Remove saved spot?" data-delete-message="This removes the saved restaurant from search. Dines with visits cannot be removed here." data-delete-confirm="Remove" onsubmit="return dinedConfirmDelete(event, this)"><input type="hidden" name="q" value="{{$.Query}}"><button class="danger history-remove-button" type="submit" aria-label="Remove {{.Restaurant.Name}} from saved spots">Remove</button></form>{{end}}</article>{{end}}</div>{{else if .Query}}<p class="empty">No saved dines match that search yet.</p>{{end}}
    {{if .Places}}<h2>Around Town</h2><div class="restaurant-list places-list">{{range .Places}}<article class="restaurant-row place-row"><div><strong>{{.DisplayName.Text}}</strong>{{if .Rating}}<em>{{score .Rating}} Google</em>{{end}}</div><span>{{.Address}}</span><span>{{if price .PriceLevel}}{{dollars (price .PriceLevel)}}{{else}}Price not listed{{end}}</span>{{if $.Authenticated}}<a class="small-cta" href="/log?restaurant_name={{query .DisplayName.Text}}&address={{query .Address}}&city={{query (placeCity .)}}&google_place_id={{query .ID}}{{if .Phone}}&phone={{query .Phone}}{{end}}{{if .Website}}&website={{query .Website}}{{end}}{{if .Rating}}&google_rating={{query (printf "%.1f" .Rating)}}{{end}}{{if price .PriceLevel}}&google_price_level={{price .PriceLevel}}{{end}}{{if .Location.Latitude}}&latitude={{query (printf "%.6f" .Location.Latitude)}}{{end}}{{if .Location.Longitude}}&longitude={{query (printf "%.6f" .Location.Longitude)}}{{end}}&category={{query (placeCategory .)}}&price_level={{price .PriceLevel}}">Log this dine</a>{{end}}</article>{{end}}</div>{{end}}
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
    {{if .Places}}<div class="restaurant-list places-list">{{range .Places}}<article class="restaurant-row place-row"><div><strong>{{.DisplayName.Text}}</strong>{{if .Rating}}<em>{{score .Rating}} Google</em>{{end}}</div><span>{{.Address}}</span><span>{{if $.HasLocation}}{{distance $.OriginLatitude $.OriginLongitude .}} · {{end}}{{if price .PriceLevel}}{{dollars (price .PriceLevel)}}{{else}}Price not listed{{end}}</span>{{if $.Authenticated}}<a class="small-cta" href="/log?restaurant_name={{query .DisplayName.Text}}&address={{query .Address}}&city={{query (placeCity .)}}&google_place_id={{query .ID}}{{if .Phone}}&phone={{query .Phone}}{{end}}{{if .Website}}&website={{query .Website}}{{end}}{{if .Rating}}&google_rating={{query (printf "%.1f" .Rating)}}{{end}}{{if price .PriceLevel}}&google_price_level={{price .PriceLevel}}{{end}}{{if .Location.Latitude}}&latitude={{query (printf "%.6f" .Location.Latitude)}}{{end}}{{if .Location.Longitude}}&longitude={{query (printf "%.6f" .Location.Longitude)}}{{end}}&category={{query (placeCategory .)}}&price_level={{price .PriceLevel}}">Log this dine</a>{{end}}</article>{{end}}</div>{{end}}
  </section>
</main>
{{template "bottom" .}}
{{end}}
`
