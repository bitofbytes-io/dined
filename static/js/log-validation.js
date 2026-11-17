// Log form validation: half-step ratings, at least one rating, and the
// submit button's enabled state.
import { syncRestaurantSelection } from "./restaurant-selection.js";
import { hasPendingPhotos, setPhotoError, PHOTO_PENDING_MESSAGE } from "./photos.js";

function validateHalfStepRating(input) {
  if (!input || input.value === "") {
    input.setCustomValidity("");
    return;
  }
  var rating = Number(input.value);
  var isHalfStep = Number.isFinite(rating) && rating >= 0 && rating <= 10 && Number.isInteger(rating * 2);
  input.setCustomValidity(isHalfStep ? "" : "Use whole numbers or .5 increments from 0 to 10.");
}

function isValidRatingValue(value) {
  if (!value || !value.trim()) return false;
  var rating = Number(value);
  return Number.isFinite(rating) && rating >= 0 && rating <= 10 && Number.isInteger(rating * 2);
}

function updateLogValidation(form) {
  if (!form) return true;
  var ratings = form.querySelectorAll("[data-half-step-rating]");
  var hasRating = false;
  for (var i = 0; i < ratings.length; i += 1) {
    validateHalfStepRating(ratings[i]);
    if (isValidRatingValue(ratings[i].value)) {
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
  var canSubmit = Boolean(hasRequiredFields && hasRating && form.checkValidity() && !hasPendingPhotos(form));
  form.classList.toggle("log-form-incomplete", !canSubmit);
  if (submit) {
    submit.disabled = !canSubmit;
    submit.setAttribute("aria-disabled", canSubmit ? "false" : "true");
  }
  return canSubmit;
}

function initializeLogValidation() {
  var ratings = document.querySelectorAll("[data-half-step-rating]");
  for (var i = 0; i < ratings.length; i += 1) {
    validateHalfStepRating(ratings[i]);
  }
  var logForms = document.querySelectorAll("[data-log-form]");
  for (var j = 0; j < logForms.length; j += 1) {
    updateLogValidation(logForms[j]);
  }
}

// Input and change both revalidate: the rating fields validate on input, and
// the form's state follows either event.
function handleFieldEvent(event) {
  if (event.target.matches("[data-half-step-rating]")) {
    validateHalfStepRating(event.target);
  }
  var logForm = event.target.closest ? event.target.closest("[data-log-form]") : null;
  if (logForm) {
    updateLogValidation(logForm);
  }
}

function handleSubmit(event) {
  var form = event.target;
  if (!form || !form.matches("[data-log-form]")) return;
  if (hasPendingPhotos(form)) {
    event.preventDefault();
    setPhotoError(form.querySelector("[data-photo-uploader]"), PHOTO_PENDING_MESSAGE);
    return;
  }
  syncRestaurantSelection(form);
  if (!updateLogValidation(form)) {
    event.preventDefault();
    if (typeof form.reportValidity === "function") {
      form.reportValidity();
    }
  }
}

document.addEventListener("input", handleFieldEvent);
document.addEventListener("change", handleFieldEvent);
document.addEventListener("photos:pending", handleFieldEvent);
document.addEventListener("submit", handleSubmit);
document.addEventListener("htmx:load", initializeLogValidation);
initializeLogValidation();
