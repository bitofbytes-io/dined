// The /dines Filter button opens the filter dialog; Close or a backdrop click closes it.
function handleClick(event) {
  var modal = document.getElementById("dines-filter-dialog");
  if (!modal || !event.target.closest) return;
  if (event.target.closest("[data-dines-filter-open]")) {
    if (typeof modal.showModal === "function") {
      if (!modal.open) modal.showModal();
    } else {
      modal.setAttribute("open", "");
    }
    return;
  }
  if (event.target === modal || event.target.closest("[data-dines-filter-close]")) {
    if (typeof modal.close === "function") {
      modal.close();
    } else {
      modal.removeAttribute("open");
    }
  }
}

// Leaves filters set to Any out of the /dines URL so filtered links stay short.
function handleSubmit(event) {
  var form = event.target;
  if (!form || !form.matches("[data-dines-filter-form]")) return;
  var params = new URLSearchParams();
  new FormData(form).forEach(function (value, name) {
    if (value !== "") params.append(name, value);
  });
  event.preventDefault();
  var query = params.toString();
  window.location.assign(form.getAttribute("action") + (query ? "?" + query : ""));
}

document.addEventListener("click", handleClick);
document.addEventListener("submit", handleSubmit);
