// The delete confirmation dialog: forms marked data-delete-form ask first,
// then submit once the dialog is confirmed.

var pendingDeleteForm = null;

function submitDeleteForm(form) {
  HTMLFormElement.prototype.submit.call(form);
}

function confirmDelete(event, form) {
  event.preventDefault();
  var modal = document.getElementById("delete-dine-modal");
  var title = form.dataset.deleteTitle || "Delete this item?";
  var message = form.dataset.deleteMessage || "This action cannot be undone.";
  var confirmText = form.dataset.deleteConfirm || "Delete";
  if (!modal || typeof modal.showModal !== "function") {
    if (confirm(title)) submitDeleteForm(form);
    return;
  }
  pendingDeleteForm = form;
  var titleNode = document.getElementById("delete-dine-title");
  var messageNode = document.getElementById("delete-dine-message");
  var confirmButton = document.getElementById("delete-dine-confirm-button");
  if (titleNode) titleNode.textContent = title;
  if (messageNode) messageNode.textContent = message;
  if (confirmButton) confirmButton.textContent = confirmText;
  if (!modal.open) modal.showModal();
}

function submitPendingDelete() {
  var form = pendingDeleteForm;
  pendingDeleteForm = null;
  if (!form) return;
  var modal = document.getElementById("delete-dine-modal");
  if (modal && modal.open) modal.close();
  submitDeleteForm(form);
}

function cancelDelete(button) {
  pendingDeleteForm = null;
  var modal = button.closest("dialog");
  if (modal && modal.open) modal.close();
}

document.addEventListener("submit", function (event) {
  var form = event.target;
  if (form && form.matches && form.matches("[data-delete-form]")) confirmDelete(event, form);
});

document.addEventListener("click", function (event) {
  if (!event.target.closest) return;
  if (event.target.closest("[data-delete-dialog-confirm]")) {
    submitPendingDelete();
  } else {
    var cancel = event.target.closest("[data-delete-dialog-cancel]");
    if (cancel) cancelDelete(cancel);
  }
});

// close does not bubble, so listen in the capture phase; the dialog itself is
// replaced when htmx swaps the page.
document.addEventListener("close", function (event) {
  if (event.target.id === "delete-dine-modal") pendingDeleteForm = null;
}, true);
