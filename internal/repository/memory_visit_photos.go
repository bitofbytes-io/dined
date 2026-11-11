package repository

import (
	"context"
	"errors"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
)

func (m *MemoryStore) VisitPhoto(_ context.Context, id uuid.UUID, variant string) (*model.VisitPhoto, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, visit := range m.visits {
		for _, photo := range visit.Photos {
			if photo.ID == id {
				copy := photo
				if variant == model.PhotoThumbnailVersion {
					copy.Bytes = photo.ThumbnailBytes
				}
				copy.Bytes = append([]byte(nil), copy.Bytes...)
				copy.ThumbnailBytes = nil
				if len(copy.Bytes) > 0 {
					copy.DataURI = ""
				}
				return &copy, nil
			}
		}
	}
	return nil, nil
}

// photoMetadata copies photos without their image data, matching list reads from Postgres.
func photoMetadata(photos []model.VisitPhoto) []model.VisitPhoto {
	if photos == nil {
		return nil
	}
	metadata := make([]model.VisitPhoto, len(photos))
	for i, photo := range photos {
		photo.DataURI = ""
		photo.Bytes = nil
		photo.ThumbnailBytes = nil
		metadata[i] = photo
	}
	return metadata
}

func (m *MemoryStore) reconciledVisitPhotos(visit model.Visit, input model.VisitInput, preparedPhotos []model.VisitPhoto) ([]model.VisitPhoto, error) {
	existingByID := map[uuid.UUID]model.VisitPhoto{}
	for _, photo := range visit.Photos {
		existingByID[photo.ID] = photo
	}

	photos := make([]model.VisitPhoto, 0, len(input.KeepPhotoIDs)+len(input.Photos))
	for order, id := range input.KeepPhotoIDs {
		photo, ok := existingByID[id]
		if !ok {
			return nil, errors.New("photo not found")
		}
		photo.SortOrder = order
		photos = append(photos, photo)
	}
	photos = append(photos, photosFromPrepared(visit.ID, preparedPhotos, len(photos))...)
	return photos, nil
}

func photosFromPrepared(visitID uuid.UUID, inputs []model.VisitPhoto, startOrder int) []model.VisitPhoto {
	photos := make([]model.VisitPhoto, 0, len(inputs))
	for i, photo := range inputs {
		photo.ID = uuid.New()
		photo.VisitID = visitID
		photo.SortOrder = startOrder + i
		photo.CreatedAt = time.Now()
		photos = append(photos, photo)
	}
	return photos
}
