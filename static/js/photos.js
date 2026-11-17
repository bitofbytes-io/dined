// Photo uploader on the log and edit forms: compresses picked images to fit
// the size limit and keeps them as hidden data URI fields.

function photoLimit(uploader) {
  var limit = uploader ? Number(uploader.dataset.photoLimit) : 0;
  return Number.isFinite(limit) && limit > 0 ? limit : 4;
}

function photoMaxBytes(uploader) {
  var maxBytes = uploader ? Number(uploader.dataset.photoMaxBytes) : 0;
  return Number.isFinite(maxBytes) && maxBytes > 0 ? maxBytes : 500 * 1024;
}

function photoCount(uploader) {
  return uploader ? uploader.querySelectorAll("[data-photo-item]").length : 0;
}

export var PHOTO_PENDING_MESSAGE = "Photos are still being prepared. Try again in a moment.";

// Photos picked but not yet compressed into tiles, per uploader. They hold a
// slot against the limit and block saving until they land or fail.
var pendingPhotos = new WeakMap();

function pendingPhotoCount(uploader) {
  return uploader ? pendingPhotos.get(uploader) || 0 : 0;
}

export function hasPendingPhotos(form) {
  var uploaders = form ? form.querySelectorAll("[data-photo-uploader]") : [];
  for (var i = 0; i < uploaders.length; i += 1) {
    if (pendingPhotoCount(uploaders[i]) > 0) return true;
  }
  return false;
}

function adjustPendingPhotos(uploader, delta) {
  var pending = Math.max(0, pendingPhotoCount(uploader) + delta);
  pendingPhotos.set(uploader, pending);
  var error = uploader.querySelector("[data-photo-error]");
  if (pending === 0 && error && error.textContent === PHOTO_PENDING_MESSAGE) {
    setPhotoError(uploader, "");
  }
  updatePhotoUploader(uploader);
  syncPhotoSubmit(uploader.closest("form"));
  // Lets the log form refresh its submit button.
  uploader.dispatchEvent(new CustomEvent("photos:pending", { bubbles: true }));
}

// Disables the save button of photo forms that don't manage it themselves
// (edit); the log form's button follows its own validation.
function syncPhotoSubmit(form) {
  if (!form || form.matches("[data-log-form]")) return;
  var submit = form.querySelector("[data-photo-submit]");
  if (!submit) return;
  var pending = hasPendingPhotos(form);
  submit.disabled = pending;
  submit.setAttribute("aria-disabled", pending ? "true" : "false");
}

export function setPhotoError(uploader, message) {
  var error = uploader ? uploader.querySelector("[data-photo-error]") : null;
  if (!error) return;
  error.textContent = message || "";
  error.hidden = !message;
}

function updatePhotoUploader(uploader) {
  if (!uploader) return;
  var addTile = uploader.querySelector("[data-photo-add]");
  var input = uploader.querySelector("[data-photo-input]");
  var count = photoCount(uploader) + pendingPhotoCount(uploader);
  var atLimit = count >= photoLimit(uploader);
  if (addTile) addTile.hidden = atLimit;
  if (input) input.disabled = atLimit;
}

function initializePhotoUploaders() {
  var uploaders = document.querySelectorAll("[data-photo-uploader]");
  for (var i = 0; i < uploaders.length; i += 1) {
    updatePhotoUploader(uploaders[i]);
    // A form restored from the htmx history cache can carry a Save button
    // disabled for photos that were pending when it was cached.
    syncPhotoSubmit(uploaders[i].closest("form"));
  }
}

function dataURIByteCount(dataURI) {
  var encoded = (dataURI.split(",", 2)[1] || "").trim();
  var padding = 0;
  if (encoded.endsWith("==")) padding = 2;
  else if (encoded.endsWith("=")) padding = 1;
  return Math.floor((encoded.length * 3) / 4) - padding;
}

function drawPhotoDataURI(image, maxDimension, quality) {
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

function compressPhoto(file, maxBytes) {
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
      // Runs outside the executor, so a throw (e.g. no 2d canvas context) must
      // reject here or the promise never settles and saving stays blocked.
      try {
        for (var i = 0; i < attempts.length; i += 1) {
          var dataURI = drawPhotoDataURI(image, attempts[i].dimension, attempts[i].quality);
          if (dataURIByteCount(dataURI) <= maxBytes) {
            resolve(dataURI);
            return;
          }
        }
      } catch (error) {
        reject(new Error("That photo could not be prepared. Try a different image."));
        return;
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

function addPhotoTile(uploader, dataURI) {
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
  updatePhotoUploader(uploader);
}

function handlePhotoInput(input) {
  var uploader = input.closest("[data-photo-uploader]");
  if (!uploader) return;
  setPhotoError(uploader, "");
  var files = Array.prototype.slice.call(input.files || []);
  var slots = photoLimit(uploader) - photoCount(uploader) - pendingPhotoCount(uploader);
  if (slots <= 0) {
    setPhotoError(uploader, "Remove a photo before adding another.");
    input.value = "";
    return;
  }
  if (files.length > slots) {
    setPhotoError(uploader, "Only " + photoLimit(uploader) + " photos can be saved for one dine.");
    files = files.slice(0, slots);
  }
  if (!files.length) return;

  adjustPendingPhotos(uploader, files.length);
  var index = 0;
  function next() {
    if (index >= files.length) {
      input.value = "";
      updatePhotoUploader(uploader);
      return;
    }
    var released = false;
    function release() {
      if (released) return;
      released = true;
      adjustPendingPhotos(uploader, -1);
    }
    compressPhoto(files[index], photoMaxBytes(uploader)).then(function (dataURI) {
      release();
      addPhotoTile(uploader, dataURI);
    }).catch(function (error) {
      release();
      setPhotoError(uploader, error.message || "Photo could not be added.");
    }).then(function () {
      index += 1;
      next();
    });
  }
  next();
}

function removePhoto(button) {
  var tile = button.closest("[data-photo-item]");
  var uploader = button.closest("[data-photo-uploader]");
  if (tile) tile.remove();
  setPhotoError(uploader, "");
  updatePhotoUploader(uploader);
}

document.addEventListener("change", function (event) {
  if (event.target.matches("[data-photo-input]")) {
    handlePhotoInput(event.target);
  }
});

document.addEventListener("click", function (event) {
  var removeButton = event.target.closest ? event.target.closest("[data-photo-remove]") : null;
  if (!removeButton) return;
  event.preventDefault();
  removePhoto(removeButton);
});

// Native submits on photo forms other than the log form (which blocks them in
// its own handler).
document.addEventListener("submit", function (event) {
  var form = event.target;
  if (!form || !form.matches || form.matches("[data-log-form]") || !hasPendingPhotos(form)) return;
  event.preventDefault();
  setPhotoError(form.querySelector("[data-photo-uploader]"), PHOTO_PENDING_MESSAGE);
});

// hx-boost forms are sent from htmx's own form-level submit handler, which runs
// before the document listeners above, so cancel the request itself.
document.addEventListener("htmx:beforeRequest", function (event) {
  var form = event.detail && event.detail.elt;
  if (!form || !form.matches || !form.matches("form") || !hasPendingPhotos(form)) return;
  event.preventDefault();
  setPhotoError(form.querySelector("[data-photo-uploader]"), PHOTO_PENDING_MESSAGE);
});

document.addEventListener("htmx:load", initializePhotoUploaders);
initializePhotoUploaders();
