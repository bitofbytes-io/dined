package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image/jpeg"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PhotoAudit contains metadata and hashes only, never image bodies or data URIs.
type PhotoAudit struct {
	ContentType      string    `json:"content_type"`
	Width            int       `json:"width"`
	Height           int       `json:"height"`
	ID               uuid.UUID `json:"id"`
	VisitID          uuid.UUID `json:"visit_id"`
	SortOrder        int       `json:"sort_order"`
	CreatedAt        time.Time `json:"created_at"`
	Bytes            int       `json:"bytes"`
	SHA256           string    `json:"sha256"`
	BinaryPresent    bool      `json:"binary_present"`
	ThumbnailPresent bool      `json:"thumbnail_present"`
}

type storedPhoto struct {
	PhotoAudit
	URI                 string
	Original, Thumbnail []byte
	Version             string
}

// SQL bounds each transferred body before it reaches the client. Invalid or
// unexpectedly large stored values become NULL and fail validation, never data
// deletion or replacement. The audit works before or after migration 008.
func auditPhotoColumns(expanded bool) string {
	columns := `id,visit_id,sort_order,created_at,byte_count,content_type,
 CASE WHEN octet_length(data_uri)<=700000 THEN data_uri ELSE '' END`
	if !expanded {
		return columns + `,NULL::bytea,NULL::bytea,''`
	}
	return columns + `,CASE WHEN octet_length(original_bytes)<=512000 THEN original_bytes END,
 CASE WHEN octet_length(thumbnail_bytes)<=512000 THEN thumbnail_bytes END,
 COALESCE(thumbnail_version,''),
 (original_bytes IS NOT NULL AND octet_length(original_bytes)>512000) OR
 (thumbnail_bytes IS NOT NULL AND octet_length(thumbnail_bytes)>512000)`
}

func scanStoredPhoto(row interface{ Scan(...any) error }, expanded bool) (storedPhoto, error) {
	var p storedPhoto
	var oversized bool
	dest := []any{&p.ID, &p.VisitID, &p.SortOrder, &p.CreatedAt, &p.Bytes, &p.ContentType, &p.URI, &p.Original, &p.Thumbnail, &p.Version}
	if expanded {
		dest = append(dest, &oversized)
	}
	if err := row.Scan(dest...); err != nil {
		return p, err
	}
	if oversized {
		return p, fmt.Errorf("photo %s has oversized binary data", p.ID)
	}
	return p, nil
}

func validateStoredPhoto(ctx context.Context, p storedPhoto) (PhotoAudit, []byte, []byte, error) {
	if p.ContentType != "image/jpeg" {
		return p.PhotoAudit, nil, nil, fmt.Errorf("photo %s has unexpected content_type", p.ID)
	}
	original, thumbnail, err := model.PrepareVisitPhoto(ctx, p.URI)
	if err != nil {
		return p.PhotoAudit, nil, nil, fmt.Errorf("photo %s: %w", p.ID, err)
	}
	if len(original) != p.Bytes {
		return p.PhotoAudit, nil, nil, fmt.Errorf("photo %s byte_count mismatch", p.ID)
	}
	if p.Original != nil && !bytes.Equal(p.Original, original) {
		return p.PhotoAudit, nil, nil, fmt.Errorf("photo %s original differs from legacy bytes", p.ID)
	}
	if p.Version == model.PhotoThumbnailVersion && p.Thumbnail != nil && !bytes.Equal(p.Thumbnail, thumbnail) {
		return p.PhotoAudit, nil, nil, fmt.Errorf("photo %s thumbnail differs from %s", p.ID, model.PhotoThumbnailVersion)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(original))
	if err != nil {
		return p.PhotoAudit, nil, nil, err
	}
	p.Width, p.Height = config.Width, config.Height
	p.SHA256 = fmt.Sprintf("%x", sha256.Sum256(original))
	p.BinaryPresent = p.Original != nil
	p.ThumbnailPresent = p.Thumbnail != nil && p.Version == model.PhotoThumbnailVersion
	return p.PhotoAudit, original, thumbnail, nil
}

// AuditPhotos validates a bounded, consistent read-only snapshot, including the
// legacy representation needed for image-only rollback. emit receives no image
// bodies. Exceeding maxRows fails before reading images; it is not a partial audit.
func (s *Store) AuditPhotos(ctx context.Context, maxRows int, emit func(PhotoAudit) error) (int, error) {
	if maxRows < 1 || maxRows > 10000 {
		return 0, fmt.Errorf("max rows must be between 1 and 10000")
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL statement_timeout='30s'; SET LOCAL lock_timeout='2s'`); err != nil {
		return 0, err
	}
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM visit_photos`).Scan(&count); err != nil {
		return 0, err
	}
	if count > maxRows {
		return 0, fmt.Errorf("%d photos exceed audit limit %d; no partial audit performed", count, maxRows)
	}
	var expanded bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='visit_photos' AND column_name='original_bytes')`).Scan(&expanded); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `SELECT `+auditPhotoColumns(expanded)+` FROM visit_photos ORDER BY id`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	for rows.Next() {
		p, err := scanStoredPhoto(rows, expanded)
		if err != nil {
			return 0, err
		}
		record, _, _, err := validateStoredPhoto(ctx, p)
		if err != nil {
			return 0, err
		}
		if err = emit(record); err != nil {
			return 0, err
		}
	}
	if err = rows.Err(); err != nil {
		return 0, err
	}
	rows.Close()
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}

const missingPhotoBytes = `(original_bytes IS NULL OR thumbnail_bytes IS NULL OR thumbnail_version IS DISTINCT FROM 'thumb-v1')`

// BackfillPhotos changes only missing binary representations, never legacy data
// or identity/ordering/timestamps. Each small batch starts at the beginning, so
// an old writer reinserting a lower UUID is not permanently missed. maxRows
// bounds a run even if old replicas keep writing; rerun after convergence.
func (s *Store) BackfillPhotos(ctx context.Context, batchSize, maxRows int) (updated, remaining int, err error) {
	if batchSize < 1 || batchSize > 100 || maxRows < 1 || maxRows > 10000 {
		return 0, 0, fmt.Errorf("batch must be 1..100 and max rows 1..10000")
	}
	processed := 0
	for processed < maxRows {
		rows, err := s.pool.Query(ctx, `SELECT id FROM visit_photos WHERE `+missingPhotoBytes+` ORDER BY id LIMIT $1`, min(batchSize, maxRows-processed))
		if err != nil {
			return updated, 0, err
		}
		var ids []uuid.UUID
		for rows.Next() {
			var id uuid.UUID
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return updated, 0, err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return updated, 0, err
		}
		if len(ids) == 0 {
			break
		}
		for _, id := range ids {
			changed, err := s.backfillPhoto(ctx, id)
			if err != nil {
				return updated, 0, err
			}
			processed++
			if changed {
				updated++
			}
		}
	}
	err = s.pool.QueryRow(ctx, `SELECT count(*) FROM visit_photos WHERE `+missingPhotoBytes).Scan(&remaining)
	return updated, remaining, err
}

func (s *Store) backfillPhoto(ctx context.Context, id uuid.UUID) (bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SET LOCAL statement_timeout='30s'; SET LOCAL lock_timeout='2s'`); err != nil {
		return false, err
	}
	p, err := scanStoredPhoto(tx.QueryRow(ctx, `SELECT `+auditPhotoColumns(true)+` FROM visit_photos WHERE id=$1 AND `+missingPhotoBytes+` FOR UPDATE`, id), true)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, original, thumbnail, err := validateStoredPhoto(ctx, p)
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE visit_photos SET original_bytes=$2,thumbnail_bytes=$3,thumbnail_version=$4 WHERE id=$1`, id, original, thumbnail, model.PhotoThumbnailVersion); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
