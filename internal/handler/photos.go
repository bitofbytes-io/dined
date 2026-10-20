package handler

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// Deleted images may remain in caches for one day; errors are never cached.
const photoCacheControl = "public, max-age=86400"

// Photo serves a stored dine photo so list pages can reference it by URL instead of embedding it.
func (h *Handler) Photo(w http.ResponseWriter, r *http.Request) {
	w = photoResponseWriter{ResponseWriter: w}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid photo ID", http.StatusBadRequest)
		return
	}
	variant := r.URL.Query().Get("variant")
	if variant != "" && variant != model.PhotoThumbnailVersion {
		http.Error(w, "Invalid photo variant", http.StatusBadRequest)
		return
	}
	photo, err := h.store.VisitPhoto(r.Context(), id, variant)
	if err != nil {
		h.error(w, "visit photo", err)
		return
	}
	if photo == nil {
		http.NotFound(w, r)
		return
	}
	image := photo.Bytes
	if len(image) == 0 {
		var original, thumbnail []byte
		original, thumbnail, err = model.PrepareVisitPhoto(r.Context(), photo.DataURI)
		image = original
		if variant == model.PhotoThumbnailVersion {
			image = thumbnail
		}
	}
	if err != nil {
		slog.Error("decode visit photo", "photo_id", id, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", photoCacheControl)
	w.Header().Set("ETag", fmt.Sprintf(`"%s-%x"`, "photo-"+variant, sha256.Sum256(image)))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(image))
}

// ServeContent removes caching headers when it generates an error (for example
// an unsatisfiable range). Set the error policy at header commit so those errors
// and our own validation/storage failures all explicitly forbid cache storage.
type photoResponseWriter struct {
	http.ResponseWriter
}

func (w photoResponseWriter) WriteHeader(status int) {
	if status >= http.StatusBadRequest {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Del("ETag")
	}
	w.ResponseWriter.WriteHeader(status)
}
