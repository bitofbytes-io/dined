package handler

import (
	"log/slog"
	"net/http"
	"strconv"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Photo IDs are never reused and kept photos retain their bytes, so a photo URL's content never changes.
const photoCacheControl = "public, max-age=604800, immutable"

// Photo serves a stored dine photo so list pages can reference it by URL instead of embedding it.
func (h *Handler) Photo(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid photo ID", http.StatusBadRequest)
		return
	}
	photo, err := h.store.VisitPhoto(r.Context(), id)
	if err != nil {
		h.error(w, "visit photo", err)
		return
	}
	if photo == nil {
		http.NotFound(w, r)
		return
	}
	image, err := model.DecodeVisitPhotoDataURI(photo.DataURI)
	if err != nil {
		slog.Error("decode visit photo", "photo_id", id, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(image)))
	w.Header().Set("Cache-Control", photoCacheControl)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(image)
}
