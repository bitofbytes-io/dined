// The photo preview dialog: opens from any [data-photo-preview] button and
// steps through the photos in the same strip or uploader.

var previewPhotos = [];
var previewIndex = 0;

function openPhotoPreview(button) {
  var source = button.dataset.photoSrc;
  if (!source) return;
  var scope = button.closest("[data-photo-strip], [data-photo-uploader]");
  var buttons = scope ? Array.prototype.slice.call(scope.querySelectorAll("[data-photo-preview]")) : [button];
  previewPhotos = buttons.map(function (item) {
    return { src: item.dataset.photoSrc || "", alt: item.dataset.photoAlt || "Dine photo" };
  }).filter(function (item) { return item.src; });
  previewIndex = Math.max(0, buttons.indexOf(button));
  if (!previewPhotos.length) return;
  var modal = document.getElementById("photo-preview-modal");
  if (!modal || typeof modal.showModal !== "function") {
    window.open(source, "_blank", "noopener");
    return;
  }
  configurePreviewModal("Dine Photo", false);
  renderPhotoPreview();
  if (!modal.open) modal.showModal();
  if (typeof modal.focus === "function") {
    modal.focus({ preventScroll: true });
  }
}

function configurePreviewModal(title, isMap) {
  var modal = document.getElementById("photo-preview-modal");
  var titleNode = document.getElementById("photo-preview-title");
  var actions = modal ? modal.querySelector(".photo-preview-actions") : null;
  if (titleNode) titleNode.textContent = title;
  if (actions) actions.hidden = Boolean(isMap);
  if (modal) modal.classList.toggle("photo-preview-modal-map", Boolean(isMap));
}

function renderPhotoPreview() {
  if (!previewPhotos.length) return;
  if (previewIndex < 0) previewIndex = previewPhotos.length - 1;
  if (previewIndex >= previewPhotos.length) previewIndex = 0;
  var photo = previewPhotos[previewIndex];
  var image = document.getElementById("photo-preview-image");
  var count = document.getElementById("photo-preview-count");
  var prev = document.getElementById("photo-preview-prev");
  var next = document.getElementById("photo-preview-next");
  if (image) {
    image.src = photo.src;
    image.alt = photo.alt;
  }
  if (count) count.textContent = (previewIndex + 1) + " / " + previewPhotos.length;
  if (prev) prev.disabled = previewPhotos.length < 2;
  if (next) next.disabled = previewPhotos.length < 2;
}

function shiftPhotoPreview(delta) {
  if (!previewPhotos.length) return;
  previewIndex += delta;
  renderPhotoPreview();
}

function closePhotoPreview() {
  var modal = document.getElementById("photo-preview-modal");
  if (modal && modal.open) modal.close();
}

document.addEventListener("click", function (event) {
  var target = event.target;
  if (!target) return;
  if (target.id === "photo-preview-modal") {
    closePhotoPreview();
    return;
  }
  if (!target.closest) return;
  if (target.closest("[data-photo-preview-close]")) {
    closePhotoPreview();
    return;
  }
  var shift = target.closest("[data-photo-preview-shift]");
  if (shift) {
    shiftPhotoPreview(Number(shift.dataset.photoPreviewShift));
    return;
  }
  var previewButton = target.closest("[data-photo-preview]");
  if (previewButton) {
    event.preventDefault();
    openPhotoPreview(previewButton);
  }
});

document.addEventListener("keydown", function (event) {
  var modal = document.getElementById("photo-preview-modal");
  if (!modal || !modal.open) return;
  if (event.key === "ArrowLeft") shiftPhotoPreview(-1);
  if (event.key === "ArrowRight") shiftPhotoPreview(1);
});
