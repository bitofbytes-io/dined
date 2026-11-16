// The trophy case map: the preview opens a larger map in a dialog.

function openTrophyMap() {
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

function closeTrophyMap() {
  var modal = document.getElementById("trophy-map-modal");
  if (modal && modal.open) modal.close();
}

document.addEventListener("click", function (event) {
  var target = event.target;
  if (!target) return;
  if (target.id === "trophy-map-modal") {
    closeTrophyMap();
    return;
  }
  if (!target.closest) return;
  if (target.closest("[data-trophy-map-open]")) {
    openTrophyMap();
  } else if (target.closest("[data-trophy-map-close]")) {
    closeTrophyMap();
  }
});
