// Nearby search: "Use my location" fills the form's coordinates from the
// browser and submits it; submitting without a place falls back to the same.

function handleUseLocationClick(event) {
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

function handleSubmit(event) {
  var form = event.target;
  if (!form || form.id !== "nearby-form") return;
  var lat = form.querySelector("input[name='lat']");
  var lng = form.querySelector("input[name='lng']");
  var near = form.querySelector("input[name='near']");
  if (!lat || !lng || (lat.value.trim() && lng.value.trim()) || (near && near.value.trim())) return;
  event.preventDefault();
  var useLocation = document.getElementById("use-location");
  if (useLocation) useLocation.click();
}

document.addEventListener("click", handleUseLocationClick);
document.addEventListener("submit", handleSubmit);
