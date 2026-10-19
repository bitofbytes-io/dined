var dinedPendingDeleteForm = null;
var dinedPreviewPhotos = [];
var dinedPreviewIndex = 0;

function dinedRestaurantOptions(value) {
  var options = document.querySelectorAll("#restaurant-options option");
  var matches = [];
  for (var i = 0; i < options.length; i += 1) {
    if (options[i].value === value) matches.push(options[i]);
  }
  return matches;
}

// The Google place a log form carries: its ID and the Places details and
// category that came with it (from a "Log this dine" link).
var dinedGooglePlaceFields = ["google_place_id", "latitude", "longitude", "phone", "website", "google_rating", "google_price_level", "category"];

function dinedGooglePlaceField(form, name) {
  return form.querySelector("[name='" + name + "']");
}

function dinedGooglePlaceValues(form) {
  var values = {};
  for (var i = 0; i < dinedGooglePlaceFields.length; i += 1) {
    var field = dinedGooglePlaceField(form, dinedGooglePlaceFields[i]);
    values[dinedGooglePlaceFields[i]] = field ? field.value : "";
  }
  return values;
}

function dinedSetGooglePlaceValues(form, values) {
  for (var i = 0; i < dinedGooglePlaceFields.length; i += 1) {
    var field = dinedGooglePlaceField(form, dinedGooglePlaceFields[i]);
    if (field) field.value = values[dinedGooglePlaceFields[i]] || "";
  }
}

// dinedPlaceOwner returns the listed restaurant whose Google place ID this is.
function dinedPlaceOwner(placeID) {
  if (!placeID) return null;
  var options = document.querySelectorAll("#restaurant-options option");
  for (var i = 0; i < options.length; i += 1) {
    if (options[i].dataset.googlePlaceId === placeID) return options[i];
  }
  return null;
}

// dinedUseRestaurantPlace runs when the user picks an existing restaurant. A
// Google place left in the form from an earlier choice belongs to the chosen
// restaurant only if it is that restaurant's place, or the restaurant has none
// and no other restaurant owns it. Otherwise the place ID and category become
// the chosen restaurant's own and the other place's details are cleared, so
// they cannot be saved onto it. The replaced values come back if the user
// un-picks it.
function dinedUseRestaurantPlace(form, option) {
  var place = form.querySelector("input[name='google_place_id']");
  if (!place) return;
  var current = place.value.trim();
  var optionPlace = option.dataset.googlePlaceId || "";
  if (current === optionPlace) return;
  if (!optionPlace) {
    var owner = dinedPlaceOwner(current);
    if (!current || !owner || owner === option) return;
  }
  if (!form.dinedReplacedPlace) form.dinedReplacedPlace = dinedGooglePlaceValues(form);
  dinedSetGooglePlaceValues(form, { google_place_id: optionPlace, category: option.dataset.category || "" });
  form.dinedReplacedPlace.applied = optionPlace;
}

// dinedReleaseRestaurantPlace restores the Google place that picking a
// restaurant replaced, unless the user has since edited the place ID.
function dinedReleaseRestaurantPlace(form) {
  var replaced = form.dinedReplacedPlace;
  if (!replaced) return;
  form.dinedReplacedPlace = null;
  var place = form.querySelector("input[name='google_place_id']");
  if (place && place.value === replaced.applied) dinedSetGooglePlaceValues(form, replaced);
}

function dinedSyncRestaurantSelection(form) {
  if (!form) return;
  var name = form.querySelector("input[name='restaurant_name']");
  var id = form.querySelector("input[name='restaurant_id']");
  var address = form.querySelector("input[name='address']");
  var city = form.querySelector("input[name='city']");
  var category = form.querySelector("select[name='category']");
  if (!name || !id) return;
  var previousID = id.value;

  var option = null;
  var options = dinedRestaurantOptions(name.value);
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
    if (previousID) dinedReleaseRestaurantPlace(form);
    return;
  }

  id.value = option.dataset.restaurantId || "";
  if (address && !address.value) address.value = optionAddress;
  if (city) city.value = option.dataset.city || "";
  if (id.value !== previousID) dinedUseRestaurantPlace(form, option);
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

function dinedPhotoMaxBytes(uploader) {
  var maxBytes = uploader ? Number(uploader.dataset.photoMaxBytes) : 0;
  return Number.isFinite(maxBytes) && maxBytes > 0 ? maxBytes : 500 * 1024;
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

function dinedCompressPhoto(file, maxBytes) {
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
        if (dinedDataURIByteCount(dataURI) <= maxBytes) {
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
  list.appendChild(tile);
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
    dinedCompressPhoto(files[index], dinedPhotoMaxBytes(uploader)).then(function (dataURI) {
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
  dinedConfigurePreviewModal("Dine Photo", false);
  dinedRenderPhotoPreview();
  if (!modal.open) modal.showModal();
  if (typeof modal.focus === "function") {
    modal.focus({ preventScroll: true });
  }
}

function dinedConfigurePreviewModal(title, isMap) {
  var modal = document.getElementById("photo-preview-modal");
  var titleNode = document.getElementById("photo-preview-title");
  var actions = modal ? modal.querySelector(".photo-preview-actions") : null;
  if (titleNode) titleNode.textContent = title;
  if (actions) actions.hidden = Boolean(isMap);
  if (modal) modal.classList.toggle("photo-preview-modal-map", Boolean(isMap));
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

function dinedOpenTrophyMap() {
  var modal = document.getElementById("trophy-map-modal");
  if (!modal || typeof modal.showModal !== "function") {
    window.open("/trophy-case/map.png", "_blank", "noopener");
    return;
  }
  if (!modal.open) modal.showModal();
  if (typeof modal.focus === "function") {
    modal.focus({ preventScroll: true });
  }
}

function dinedCloseTrophyMap() {
  var modal = document.getElementById("trophy-map-modal");
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
  if (event.target && event.target.id === "trophy-map-modal") {
    dinedCloseTrophyMap();
    return;
  }
  if (dinedHandleDinesFilterClick(event)) return;
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

// The /dines Filter button opens the filter dialog; Close or a backdrop click closes it.
function dinedHandleDinesFilterClick(event) {
  var modal = document.getElementById("dines-filter-dialog");
  if (!modal || !event.target.closest) return false;
  if (event.target.closest("[data-dines-filter-open]")) {
    if (typeof modal.showModal === "function") {
      if (!modal.open) modal.showModal();
    } else {
      modal.setAttribute("open", "");
    }
    return true;
  }
  if (event.target === modal || event.target.closest("[data-dines-filter-close]")) {
    if (typeof modal.close === "function") {
      modal.close();
    } else {
      modal.removeAttribute("open");
    }
    return true;
  }
  return false;
}

// Leaves filters set to Any out of the /dines URL so filtered links stay short.
function dinedSubmitDinesFilter(event, form) {
  var params = new URLSearchParams();
  new FormData(form).forEach(function (value, name) {
    if (value !== "") params.append(name, value);
  });
  event.preventDefault();
  var query = params.toString();
  window.location.assign(form.getAttribute("action") + (query ? "?" + query : ""));
}

function dinedHandleSubmit(event) {
  var form = event.target;
  if (form && form.matches("[data-dines-filter-form]")) {
    dinedSubmitDinesFilter(event, form);
    return;
  }
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

// Want to Try: the save sheet behind "+ Want to Try" links and the list
// page's "Pick one for us". Each link's href is the save form page, which
// browsers without the sheet open instead; its query names the place.
var dinedWantToTrySource = null;
// dinedWantToTryOpening counts sheet openings, so a save answers only the
// opening it was sent from and cannot close or flag a later one.
var dinedWantToTryOpening = 0;

function dinedWantToTrySheet() {
  return document.getElementById("want-to-try-sheet");
}

function dinedSetWantToTryError(form, message) {
  var error = form.querySelector("[data-want-to-try-sheet-error]");
  if (!error) return;
  error.textContent = message || "";
  error.hidden = !message;
}

function dinedOpenWantToTrySheet(link) {
  var sheet = dinedWantToTrySheet();
  var form = sheet ? sheet.querySelector("[data-want-to-try-sheet-form]") : null;
  if (!form || typeof sheet.showModal !== "function") return false;
  form.reset();
  var params = new URL(link.href, window.location.href).searchParams;
  var fields = form.querySelectorAll("[data-want-to-try-field]");
  for (var i = 0; i < fields.length; i += 1) {
    fields[i].value = params.get(fields[i].name) || "";
  }
  var name = sheet.querySelector("[data-want-to-try-sheet-name]");
  var address = sheet.querySelector("[data-want-to-try-sheet-address]");
  if (name) name.textContent = link.dataset.wantToTryName || "";
  if (address) {
    address.textContent = link.dataset.wantToTryAddress || "";
    address.hidden = !address.textContent;
  }
  dinedSetWantToTryError(form, "");
  var save = form.querySelector("[data-want-to-try-save]");
  if (save) save.disabled = false;
  dinedWantToTrySource = link;
  dinedWantToTryOpening += 1;
  if (!sheet.open) sheet.showModal();
  return true;
}

function dinedCloseWantToTrySheet() {
  var sheet = dinedWantToTrySheet();
  if (sheet && sheet.open) sheet.close();
  dinedWantToTrySource = null;
}

function dinedMarkWantToTrySaved(link) {
  if (!link || !link.parentNode) return;
  var saved = document.createElement("span");
  saved.className = "want-on";
  saved.textContent = "✓ On Want to Try";
  link.replaceWith(saved);
}

function dinedSubmitWantToTry(form) {
  var save = form.querySelector("[data-want-to-try-save]");
  var source = dinedWantToTrySource;
  var opening = dinedWantToTryOpening;
  function current() {
    var sheet = dinedWantToTrySheet();
    return opening === dinedWantToTryOpening && sheet && sheet.open;
  }
  function fail(message) {
    if (current()) dinedSetWantToTryError(form, message);
  }
  if (save) save.disabled = true;
  dinedSetWantToTryError(form, "");
  fetch(form.action, {
    method: "POST",
    credentials: "same-origin",
    headers: { "X-Dined-Want-To-Try-Sheet": "1" },
    body: new URLSearchParams(new FormData(form))
  }).then(function (response) {
    if (response.ok) {
      dinedMarkWantToTrySaved(source);
      if (current()) dinedCloseWantToTrySheet();
      return null;
    }
    if (response.status === 401) {
      fail("Log in again to save places.");
      return null;
    }
    return response.text().then(function (text) {
      fail(text.trim() || "Could not save this place. Please try again.");
    });
  }).catch(function () {
    fail("Could not save this place. Check your connection and try again.");
  }).then(function () {
    if (save && opening === dinedWantToTryOpening) save.disabled = false;
  });
}

function dinedPickWantToTry(button) {
  var rows = Array.prototype.slice.call(document.querySelectorAll("[data-want-to-try-row]"));
  if (!rows.length) return;
  var choices = rows.filter(function (row) { return !row.classList.contains("is-picked"); });
  if (!choices.length) choices = rows;
  var picked = choices[Math.floor(Math.random() * choices.length)];
  for (var i = 0; i < rows.length; i += 1) {
    rows[i].classList.toggle("is-picked", rows[i] === picked);
  }
  var result = document.querySelector("[data-want-to-try-pick-result]");
  if (result) {
    result.textContent = "How about " + (picked.dataset.wantToTryName || "this one") + "?";
    result.hidden = false;
  }
  button.textContent = "Pick again";
}

function dinedHandleWantToTryClick(event) {
  var target = event.target;
  if (!target || !target.closest) return;
  var open = target.closest("[data-want-to-try-open]");
  if (open) {
    if (dinedOpenWantToTrySheet(open)) event.preventDefault();
    return;
  }
  if (target.closest("[data-want-to-try-cancel]") || target === dinedWantToTrySheet()) {
    dinedCloseWantToTrySheet();
    return;
  }
  var pick = target.closest("[data-want-to-try-pick]");
  if (pick) dinedPickWantToTry(pick);
}

function dinedHandleWantToTrySubmit(event) {
  var form = event.target;
  if (!form || !form.matches || !form.matches("[data-want-to-try-sheet-form]")) return;
  event.preventDefault();
  dinedSubmitWantToTry(form);
}

function dinedInitializeWantToTry() {
  var pickAreas = document.querySelectorAll("[data-want-to-try-pick-area]");
  for (var i = 0; i < pickAreas.length; i += 1) {
    pickAreas[i].hidden = false;
  }
  var result = document.querySelector("[data-want-to-try-pick-result]");
  if (result && !result.textContent) result.hidden = true;
}

if (!window.dinedWantToTryListenersBound) {
  window.dinedWantToTryListenersBound = true;
  document.addEventListener("click", dinedHandleWantToTryClick);
  document.addEventListener("submit", dinedHandleWantToTrySubmit);
  document.addEventListener("htmx:load", dinedInitializeWantToTry);
}
dinedInitializeWantToTry();

// Where should we eat?: the result card flashes through a few other names
// from the hat, slowing down, before it settles on the pick.
function dinedRunSpinReel() {
  var card = document.querySelector("[data-spin-result]");
  if (!card || card.dataset.spinReelDone) return;
  card.dataset.spinReelDone = "1";
  var name = card.querySelector("[data-spin-name]");
  var names = [];
  try {
    names = JSON.parse(card.dataset.spinReel || "[]");
  } catch (error) {
    names = [];
  }
  var reduceMotion = window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  if (!name || !names.length || reduceMotion) return;
  var pick = name.textContent;
  var frames = names.length < 6 ? names.concat(names) : names;
  var frame = 0;
  card.classList.add("is-spinning");
  card.setAttribute("aria-busy", "true");
  function step() {
    if (frame < frames.length) {
      name.textContent = frames[frame];
      frame += 1;
      window.setTimeout(step, 45 + frame * 12);
      return;
    }
    name.textContent = pick;
    card.classList.remove("is-spinning");
    card.removeAttribute("aria-busy");
  }
  step();
}

if (!window.dinedSpinListenersBound) {
  window.dinedSpinListenersBound = true;
  document.addEventListener("htmx:load", dinedRunSpinReel);
}
dinedRunSpinReel();
