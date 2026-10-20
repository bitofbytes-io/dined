package repository

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func photoAudit(t *testing.T, s *Store) []PhotoAudit {
	t.Helper()
	var records []PhotoAudit
	count, err := s.AuditPhotos(context.Background(), 1000, func(p PhotoAudit) error { records = append(records, p); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if count != len(records) {
		t.Fatal("incomplete audit")
	}
	return records
}

func TestPostgresPhotoBackfillPreservesOldWriterRows(t *testing.T) {
	s := postgresStore(t)
	ctx := context.Background()
	input := postgresVisitInput(t, s, "Photo compatibility")
	input.Photos = []model.VisitPhotoInput{{DataURI: testutil.PhotoURI(1)}, {DataURI: testutil.PhotoURI(2)}}
	id, err := s.CreateVisit(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	baseline := photoAudit(t, s)
	// Reproduce the previous binary's delete/reinsert behavior: all newly added
	// columns return to NULL, while legacy identity and metadata stay intact.
	oldRewrite := func() {
		t.Helper()
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		rows, err := tx.Query(ctx, `SELECT id,visit_id,data_uri,content_type,byte_count,sort_order,created_at FROM visit_photos WHERE visit_id=$1 ORDER BY sort_order,created_at,id`, *id)
		if err != nil {
			t.Fatal(err)
		}
		photos, err := scanVisitPhotos(rows)
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `DELETE FROM visit_photos WHERE visit_id=$1`, *id); err != nil {
			t.Fatal(err)
		}
		for _, p := range photos {
			if _, err = tx.Exec(ctx, `INSERT INTO visit_photos(id,visit_id,data_uri,content_type,byte_count,sort_order,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, p.ID, p.VisitID, p.DataURI, p.ContentType, p.ByteCount, p.SortOrder, p.CreatedAt); err != nil {
				t.Fatal(err)
			}
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	oldRewrite()
	for _, p := range baseline {
		for _, variant := range []string{"", model.PhotoThumbnailVersion} {
			got, err := s.VisitPhoto(ctx, p.ID, variant)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Bytes) != 0 || got.DataURI == "" {
				t.Fatal("old writer row lost fallback")
			}
			original, thumbnail, err := model.PrepareVisitPhoto(ctx, got.DataURI)
			if err != nil {
				t.Fatal(err)
			}
			if len(original) != p.Bytes || len(thumbnail) == 0 {
				t.Fatal("invalid fallback")
			}
		}
	}
	updated, remaining, err := s.BackfillPhotos(ctx, 1, 1)
	if err != nil || updated != 1 || remaining != 1 {
		t.Fatalf("bounded pass=%d remaining=%d err=%v", updated, remaining, err)
	}
	updated, remaining, err = s.BackfillPhotos(ctx, 1, 10)
	if err != nil || updated != 1 || remaining != 0 {
		t.Fatalf("resume=%d remaining=%d err=%v", updated, remaining, err)
	}
	updated, remaining, err = s.BackfillPhotos(ctx, 1, 10)
	if err != nil || updated != 0 || remaining != 0 {
		t.Fatalf("idempotence=%d remaining=%d err=%v", updated, remaining, err)
	}
	// Rows with UUIDs behind an earlier batch are rediscovered after rollback writes.
	oldRewrite()
	updated, remaining, err = s.BackfillPhotos(ctx, 1, 10)
	if err != nil || updated != 2 || remaining != 0 {
		t.Fatalf("old writer reinsert=%d remaining=%d err=%v", updated, remaining, err)
	}
	after := photoAudit(t, s)
	for i, want := range baseline {
		got := after[i]
		if got != want {
			t.Fatalf("audit changed: got %+v want %+v", got, want)
		}
	}
	visit, err := s.Visit(ctx, *id)
	if err != nil {
		t.Fatal(err)
	}
	update := input
	update.Photos = nil
	update.RestaurantID = &visit.Restaurant.ID
	update.KeepPhotoIDs = []uuid.UUID{visit.Photos[1].ID, visit.Photos[0].ID}
	if err = s.UpdateVisit(ctx, *id, update); err != nil {
		t.Fatal(err)
	}
	for _, p := range photoAudit(t, s) {
		var before PhotoAudit
		for _, b := range baseline {
			if b.ID == p.ID {
				before = b
			}
		}
		if p.SHA256 != before.SHA256 || !p.CreatedAt.Equal(before.CreatedAt) || !p.BinaryPresent || !p.ThumbnailPresent {
			t.Fatal("retained photo rewritten")
		}
		photo, err := s.VisitPhoto(ctx, p.ID, "")
		if err != nil || len(photo.Bytes) == 0 || photo.DataURI != "" {
			t.Fatal("binary read failed", err)
		}
	}
	// No resurrection after a concurrent deletion between selection and lock.
	if err = s.DeleteVisit(ctx, *id); err != nil {
		t.Fatal(err)
	}
	if changed, err := s.backfillPhoto(ctx, baseline[0].ID); err != nil || changed {
		t.Fatal("deleted photo resurrected", err)
	}
}

func TestPostgresPhotoAuditFailsClosedAndDoesNotWrite(t *testing.T) {
	s := postgresStore(t)
	ctx := context.Background()
	input := postgresVisitInput(t, s, "Audit failure")
	input.Photos = []model.VisitPhotoInput{{DataURI: testutil.PhotoURI(1)}}
	id, err := s.CreateVisit(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	visit, err := s.Visit(ctx, *id)
	if err != nil {
		t.Fatal(err)
	}
	photoID := visit.Photos[0].ID
	for _, tc := range []struct{ name, sql string }{
		{"invalid legacy", `UPDATE visit_photos SET data_uri='data:image/jpeg;base64,aGVsbG8=',original_bytes=NULL,thumbnail_bytes=NULL,thumbnail_version=NULL`},
		{"byte count", `UPDATE visit_photos SET byte_count=1`},
		{"mismatched binary", `UPDATE visit_photos SET original_bytes=decode('0102','hex')`},
		{"mismatched thumbnail", `UPDATE visit_photos SET thumbnail_bytes=decode('0102','hex')`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original, thumbnail, _ := model.PrepareVisitPhoto(ctx, testutil.PhotoURI(1))
			if _, err = s.pool.Exec(ctx, `UPDATE visit_photos SET data_uri=$1,byte_count=$2,original_bytes=$3,thumbnail_bytes=$4,thumbnail_version=$5`, testutil.PhotoURI(1), len(original), original, thumbnail, model.PhotoThumbnailVersion); err != nil {
				t.Fatal(err)
			}
			if _, err = s.pool.Exec(ctx, tc.sql); err != nil {
				t.Fatal(err)
			}
			var before string
			if err = s.pool.QueryRow(ctx, `SELECT row_to_json(p)::text FROM visit_photos p WHERE id=$1`, photoID).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if _, err = s.AuditPhotos(ctx, 10, func(PhotoAudit) error { return nil }); err == nil {
				t.Fatal("audit accepted corrupt image")
			}
			var after string
			if err = s.pool.QueryRow(ctx, `SELECT row_to_json(p)::text FROM visit_photos p WHERE id=$1`, photoID).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if before != after {
				t.Fatal("read-only audit modified data")
			}
			if tc.name == "invalid legacy" {
				if _, _, err = s.BackfillPhotos(ctx, 1, 10); err == nil {
					t.Fatal("backfill accepted corrupt legacy")
				}
			}
		})
	}
}

func TestPostgresPhotoAuditBeforeAdditiveSchema(t *testing.T) {
	s := postgresStore(t)
	ctx := context.Background()
	input := postgresVisitInput(t, s, "Preflight")
	input.Photos = []model.VisitPhotoInput{{DataURI: testutil.PhotoURI(1)}}
	_, err := s.CreateVisit(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	// Test only: reconstruct the previous schema to exercise a pre-deploy audit.
	if _, err = s.pool.Exec(ctx, `ALTER TABLE visit_photos DROP COLUMN original_bytes,DROP COLUMN thumbnail_bytes,DROP COLUMN thumbnail_version`); err != nil {
		t.Fatal(err)
	}
	records := photoAudit(t, s)
	if len(records) != 1 || records[0].BinaryPresent || records[0].ThumbnailPresent || records[0].SHA256 == "" {
		t.Fatal("bad legacy audit", records)
	}
	if _, err = s.AuditPhotos(ctx, 0, func(PhotoAudit) error { return nil }); err == nil {
		t.Fatal("unbounded audit allowed")
	}
}

func TestPostgresPhotoBinarySelectionAndMetadata(t *testing.T) {
	s := postgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw := testutil.JPEG(1280, 960, 1)
	input := postgresVisitInput(t, s, "Binary selection")
	input.Photos = []model.VisitPhotoInput{{DataURI: testutil.URI(raw)}}
	id, err := s.CreateVisit(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	visit, err := s.Visit(ctx, *id)
	if err != nil {
		t.Fatal(err)
	}
	meta := visit.Photos[0]
	if meta.DataURI != "" || meta.Bytes != nil || meta.ThumbnailBytes != nil {
		t.Fatal("visit loaded image bodies")
	}
	original, err := s.VisitPhoto(ctx, meta.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	thumbnail, err := s.VisitPhoto(ctx, meta.ID, model.PhotoThumbnailVersion)
	if err != nil {
		t.Fatal(err)
	}
	_, wantThumb, err := model.PrepareVisitPhoto(ctx, testutil.URI(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original.Bytes, raw) || !bytes.Equal(thumbnail.Bytes, wantThumb) || original.DataURI != "" || thumbnail.DataURI != "" {
		t.Fatal("wrong representation")
	}
	if len(thumbnail.Bytes) >= len(original.Bytes) {
		t.Fatalf("thumbnail not smaller: %d >= %d", len(thumbnail.Bytes), len(original.Bytes))
	}
	if _, err = s.AuditPhotos(ctx, 1, func(PhotoAudit) error { return fmt.Errorf("output stopped") }); err == nil {
		t.Fatal("audit ignored output failure")
	}
}

func TestPostgresPhotoBackfillRespectsConcurrentRowLock(t *testing.T) {
	s := postgresStore(t)
	ctx := context.Background()
	input := postgresVisitInput(t, s, "Concurrent writer")
	input.Photos = []model.VisitPhotoInput{{DataURI: testutil.PhotoURI(1)}}
	_, err := s.CreateVisit(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	record := photoAudit(t, s)[0]
	if _, err = s.pool.Exec(ctx, `UPDATE visit_photos SET original_bytes=NULL,thumbnail_bytes=NULL,thumbnail_version=NULL`); err != nil {
		t.Fatal(err)
	}
	cfg := s.pool.Config()
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	concurrent := New(pool)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT id FROM visit_photos WHERE id=$1 FOR UPDATE`, record.ID); err != nil {
		t.Fatal(err)
	}
	blocked, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	if changed, err := concurrent.backfillPhoto(blocked, record.ID); err == nil || changed {
		t.Fatal("backfill bypassed concurrent row lock", err)
	}
	// The old writer commits a deletion; the waiting/retried backfill must not
	// recreate the selected row or attach its data to another row.
	if _, err = tx.Exec(ctx, `DELETE FROM visit_photos WHERE id=$1`, record.ID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if changed, err := concurrent.backfillPhoto(ctx, record.ID); err != nil || changed {
		t.Fatal("deleted row resurrected", err)
	}
	if records := photoAudit(t, s); len(records) != 0 {
		t.Fatal("photo should remain deleted")
	}
}
