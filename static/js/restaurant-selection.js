// Picking an existing restaurant on a log form: the datalist match fills in
// the restaurant ID, address, city and category, and carries its Google place.

function restaurantOptions(value) {
  var options = document.querySelectorAll("#restaurant-options option");
  var matches = [];
  for (var i = 0; i < options.length; i += 1) {
    if (options[i].value === value) matches.push(options[i]);
  }
  return matches;
}

// The Google place a log form carries: its ID and the Places details and
// category that came with it (from a "Log this dine" link).
var googlePlaceFields = ["google_place_id", "latitude", "longitude", "phone", "website", "google_rating", "google_price_level", "category"];

function googlePlaceField(form, name) {
  return form.querySelector("[name='" + name + "']");
}

function googlePlaceValues(form) {
  var values = {};
  for (var i = 0; i < googlePlaceFields.length; i += 1) {
    var field = googlePlaceField(form, googlePlaceFields[i]);
    values[googlePlaceFields[i]] = field ? field.value : "";
  }
  return values;
}

function setGooglePlaceValues(form, values) {
  for (var i = 0; i < googlePlaceFields.length; i += 1) {
    var field = googlePlaceField(form, googlePlaceFields[i]);
    if (field) field.value = values[googlePlaceFields[i]] || "";
  }
}

// placeOwner returns the listed restaurant whose Google place ID this is.
function placeOwner(placeID) {
  if (!placeID) return null;
  var options = document.querySelectorAll("#restaurant-options option");
  for (var i = 0; i < options.length; i += 1) {
    if (options[i].dataset.googlePlaceId === placeID) return options[i];
  }
  return null;
}

// useRestaurantPlace runs when the user picks an existing restaurant. A
// Google place left in the form from an earlier choice belongs to the chosen
// restaurant only if it is that restaurant's place, or the restaurant has none
// and no other restaurant owns it. Otherwise the place ID and category become
// the chosen restaurant's own and the other place's details are cleared, so
// they cannot be saved onto it. The replaced values come back if the user
// un-picks it.
function useRestaurantPlace(form, option) {
  var place = form.querySelector("input[name='google_place_id']");
  if (!place) return;
  var current = place.value.trim();
  var optionPlace = option.dataset.googlePlaceId || "";
  if (current === optionPlace) return;
  if (!optionPlace) {
    var owner = placeOwner(current);
    if (!current || !owner || owner === option) return;
  }
  if (!form.dinedReplacedPlace) form.dinedReplacedPlace = googlePlaceValues(form);
  setGooglePlaceValues(form, { google_place_id: optionPlace, category: option.dataset.category || "" });
  form.dinedReplacedPlace.applied = optionPlace;
}

// releaseRestaurantPlace restores the Google place that picking a
// restaurant replaced, unless the user has since edited the place ID.
function releaseRestaurantPlace(form) {
  var replaced = form.dinedReplacedPlace;
  if (!replaced) return;
  form.dinedReplacedPlace = null;
  var place = form.querySelector("input[name='google_place_id']");
  if (place && place.value === replaced.applied) setGooglePlaceValues(form, replaced);
}

export function syncRestaurantSelection(form) {
  if (!form) return;
  var name = form.querySelector("input[name='restaurant_name']");
  var id = form.querySelector("input[name='restaurant_id']");
  var address = form.querySelector("input[name='address']");
  var city = form.querySelector("input[name='city']");
  var category = form.querySelector("select[name='category']");
  if (!name || !id) return;
  var previousID = id.value;

  var option = null;
  var options = restaurantOptions(name.value);
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
  var optionAddress = option ? option.dataset.address || "" : "";
  if (option && address && address.value && optionAddress && address.value !== optionAddress) {
    option = null;
  }
  if (!option) {
    id.value = "";
    if (previousID) releaseRestaurantPlace(form);
    return;
  }

  id.value = option.dataset.restaurantId || "";
  if (address && !address.value) address.value = optionAddress;
  if (city) city.value = option.dataset.city || "";
  if (id.value !== previousID) useRestaurantPlace(form, option);
  if (category && !category.value) category.value = option.dataset.category || "";
}

document.addEventListener("input", function (event) {
  if (event.target.name === "restaurant_name" || event.target.name === "address") {
    syncRestaurantSelection(event.target.form);
  }
});
