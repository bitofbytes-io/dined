package handler

import (
	"bytes"
	"log/slog"
	"net/http"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// A photo ID's bytes never change, so the ID is a strong ETag. The short max-age bounds how long a
// deleted photo can still be served from a cache; after that, clients revalidate and get a 304 or 404.
const photoCacheControl = "public, max-age=300"

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
	w.Header().Set("Cache-Control", photoCacheControl)
	w.Header().Set("ETag", `"`+photo.ID.String()+`"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(image))
}
