// Want to Try: the save sheet behind "+ Want to Try" links and the list
// page's "Pick one for us". Each link's href is the save form page, which
// browsers without the sheet open instead; its query names the place.
var wantToTrySource = null;
// wantToTryOpening counts sheet openings, so a save answers only the
// opening it was sent from and cannot close or flag a later one.
var wantToTryOpening = 0;

function wantToTrySheet() {
  return document.getElementById("want-to-try-sheet");
}

function setWantToTryError(form, message) {
  var error = form.querySelector("[data-want-to-try-sheet-error]");
  if (!error) return;
  error.textContent = message || "";
  error.hidden = !message;
}

function openWantToTrySheet(link) {
  var sheet = wantToTrySheet();
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
  setWantToTryError(form, "");
  var save = form.querySelector("[data-want-to-try-save]");
  if (save) save.disabled = false;
  wantToTrySource = link;
  wantToTryOpening += 1;
  if (!sheet.open) sheet.showModal();
  return true;
}

function closeWantToTrySheet() {
  var sheet = wantToTrySheet();
  if (sheet && sheet.open) sheet.close();
  wantToTrySource = null;
}

function markWantToTrySaved(link) {
  if (!link || !link.parentNode) return;
  var saved = document.createElement("span");
  saved.className = "want-on";
  saved.textContent = "✓ On Want to Try";
  link.replaceWith(saved);
}

function submitWantToTry(form) {
  var save = form.querySelector("[data-want-to-try-save]");
  var source = wantToTrySource;
  var opening = wantToTryOpening;
  function current() {
    var sheet = wantToTrySheet();
    return opening === wantToTryOpening && sheet && sheet.open;
  }
  function fail(message) {
    if (current()) setWantToTryError(form, message);
  }
  if (save) save.disabled = true;
  setWantToTryError(form, "");
  fetch(form.action, {
    method: "POST",
    credentials: "same-origin",
    headers: { "X-Dined-Want-To-Try-Sheet": "1" },
    body: new URLSearchParams(new FormData(form))
  }).then(function (response) {
    if (response.ok) {
      markWantToTrySaved(source);
      if (current()) closeWantToTrySheet();
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
    if (save && opening === wantToTryOpening) save.disabled = false;
  });
}

function pickWantToTry(button) {
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

function handleClick(event) {
  var target = event.target;
  if (!target || !target.closest) return;
  var open = target.closest("[data-want-to-try-open]");
  if (open) {
    if (openWantToTrySheet(open)) event.preventDefault();
    return;
  }
  if (target.closest("[data-want-to-try-cancel]") || target === wantToTrySheet()) {
    closeWantToTrySheet();
    return;
  }
  var pick = target.closest("[data-want-to-try-pick]");
  if (pick) pickWantToTry(pick);
}

function handleSubmit(event) {
  var form = event.target;
  if (!form || !form.matches || !form.matches("[data-want-to-try-sheet-form]")) return;
  event.preventDefault();
  submitWantToTry(form);
}

function initializeWantToTry() {
  var pickAreas = document.querySelectorAll("[data-want-to-try-pick-area]");
  for (var i = 0; i < pickAreas.length; i += 1) {
    pickAreas[i].hidden = false;
  }
  var result = document.querySelector("[data-want-to-try-pick-result]");
  if (result && !result.textContent) result.hidden = true;
}

document.addEventListener("click", handleClick);
document.addEventListener("submit", handleSubmit);
document.addEventListener("htmx:load", initializeWantToTry);
initializeWantToTry();
