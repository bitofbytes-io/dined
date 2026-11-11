package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// VisitPhoto fetches only the selected representation. Old-writer rows retain
// their legacy URI until an explicit backfill; no list query loads image bodies.
func (s *Store) VisitPhoto(ctx context.Context, id uuid.UUID, variant string) (*model.VisitPhoto, error) {
	var photo model.VisitPhoto
	err := s.pool.QueryRow(ctx, `
 SELECT id, visit_id, content_type, byte_count, sort_order, created_at,
 CASE WHEN $2 = $3 THEN
   CASE WHEN thumbnail_version = $2 THEN thumbnail_bytes END
 ELSE original_bytes END,
 CASE WHEN ($2 = $3 AND (thumbnail_bytes IS NULL OR thumbnail_version IS DISTINCT FROM $2))
   OR ($2 <> $3 AND original_bytes IS NULL) THEN data_uri ELSE '' END
 FROM visit_photos WHERE id = $1`, id, variant, model.PhotoThumbnailVersion).Scan(
		&photo.ID, &photo.VisitID, &photo.ContentType, &photo.ByteCount, &photo.SortOrder, &photo.CreatedAt, &photo.Bytes, &photo.DataURI)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("visit photo: %w", err)
	}
	return &photo, nil
}

func insertVisitPhotos(ctx context.Context, tx pgx.Tx, visitID uuid.UUID, photos []model.VisitPhoto, startOrder int) error {
	for i, photo := range photos {
		_, err := tx.Exec(ctx, `INSERT INTO visit_photos
  (visit_id,data_uri,content_type,byte_count,sort_order,original_bytes,thumbnail_bytes,thumbnail_version)
  VALUES ($1,$2,'image/jpeg',$3,$4,$5,$6,$7)`, visitID, photo.DataURI, photo.ByteCount, startOrder+i, photo.Bytes, photo.ThumbnailBytes, model.PhotoThumbnailVersion)
		if err != nil {
			return err
		}
	}
	return nil
}

func reconcileVisitPhotos(ctx context.Context, tx pgx.Tx, visitID uuid.UUID, input model.VisitInput, photos []model.VisitPhoto) error {
	// Validate ownership before deleting anything. Retained rows are updated in
	// place: their IDs, creation times, legacy URI, and binary bytes never change.
	for _, id := range input.KeepPhotoIDs {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM visit_photos WHERE visit_id=$1 AND id=$2)`, visitID, id).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return errors.New("photo not found")
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM visit_photos WHERE visit_id=$1 AND NOT (id=ANY($2::uuid[]))`, visitID, nonNilPhotoIDs(input.KeepPhotoIDs)); err != nil {
		return err
	}
	for order, id := range input.KeepPhotoIDs {
		if _, err := tx.Exec(ctx, `UPDATE visit_photos SET sort_order=$3 WHERE visit_id=$1 AND id=$2`, visitID, id, order); err != nil {
			return err
		}
	}
	return insertVisitPhotos(ctx, tx, visitID, photos, len(input.KeepPhotoIDs))
}

func nonNilPhotoIDs(ids []uuid.UUID) []uuid.UUID {
	if ids == nil {
		return []uuid.UUID{}
	}
	return ids
}

// loadVisitPhotos attaches metadata only; image bodies are fetched by the photo endpoint.
func (s *Store) loadVisitPhotos(ctx context.Context, ids []uuid.UUID, byID map[uuid.UUID]*model.Visit) error {
	rows, err := s.pool.Query(ctx, `
		SELECT id, visit_id, '', content_type, byte_count, sort_order, created_at
		FROM visit_photos
		WHERE visit_id = ANY($1)
		ORDER BY sort_order, created_at, id`, ids)
	if err != nil {
		return fmt.Errorf("list visit photos: %w", err)
	}
	defer rows.Close()
	photos, err := scanVisitPhotos(rows)
	if err != nil {
		return fmt.Errorf("scan visit photos: %w", err)
	}
	for _, photo := range photos {
		if visit := byID[photo.VisitID]; visit != nil {
			visit.Photos = append(visit.Photos, photo)
		}
	}
	return nil
}

func scanVisitPhotos(rows pgx.Rows) ([]model.VisitPhoto, error) {
	var photos []model.VisitPhoto
	for rows.Next() {
		var photo model.VisitPhoto
		if err := rows.Scan(
			&photo.ID,
			&photo.VisitID,
			&photo.DataURI,
			&photo.ContentType,
			&photo.ByteCount,
			&photo.SortOrder,
			&photo.CreatedAt,
		); err != nil {
			return nil, err
		}
		photos = append(photos, photo)
	}
	return photos, rows.Err()
}
