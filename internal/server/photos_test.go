package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/model"
	"github.com/bitofbytes-io/dined/internal/repository"
	"github.com/bitofbytes-io/dined/internal/testutil"
	"github.com/google/uuid"
)

type legacyPhotoStore struct {
	repository.DinerStore
	photo model.VisitPhoto
	err   error
}

func (s legacyPhotoStore) VisitPhoto(_ context.Context, id uuid.UUID, _ string) (*model.VisitPhoto, error) {
	if s.err != nil {
		return nil, s.err
	}
	if id != s.photo.ID {
		return nil, nil
	}
	return &s.photo, nil
}

func TestPhotoVariantsServeExactBytesAndCacheSemantics(t *testing.T) {
	ctx := context.Background()
	store := repository.NewMemoryStore()
	people, _ := store.People(ctx)
	original := testutil.JPEG(1280, 960, 3)
	raw := testutil.URI(original)
	_, thumbnail, err := model.PrepareVisitPhoto(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	visitID, err := store.CreateVisit(ctx, model.VisitInput{RestaurantName: "Variant diner", VisitedAt: time.Now(), PriceLevel: 2, Ratings: map[uuid.UUID]float64{people[0].ID: 8}, Photos: []model.VisitPhotoInput{{DataURI: raw}}})
	if err != nil {
		t.Fatal(err)
	}
	visit, err := store.Visit(ctx, *visitID)
	if err != nil {
		t.Fatal(err)
	}
	photoID := visit.Photos[0].ID
	legacy := legacyPhotoStore{DinerStore: store, photo: model.VisitPhoto{ID: photoID, DataURI: raw}}
	for name, backend := range map[string]repository.DinerStore{"binary": store, "old writer": legacy} {
		t.Run(name, func(t *testing.T) {
			router, _ := newAuthenticatedTestRouter(t, backend)
			for _, variant := range []string{"", model.PhotoThumbnailVersion} {
				t.Run("variant="+variant, func(t *testing.T) {
					want := original
					if variant != "" {
						want = thumbnail
					}
					url := "/photos/" + photoID.String() + "?variant=" + variant
					etag := fmt.Sprintf(`"photo-%s-%x"`, variant, sha256.Sum256(want))
					other := fmt.Sprintf(`"photo-%s-%x"`, model.PhotoThumbnailVersion, sha256.Sum256(thumbnail))
					if variant != "" {
						other = fmt.Sprintf(`"photo--%x"`, sha256.Sum256(original))
					}
					for _, tc := range []struct {
						name, method string
						headers      map[string]string
						status       int
						body         []byte
					}{
						{name: "get", method: "GET", status: 200, body: want},
						{name: "head", method: "HEAD", status: 200},
						{name: "304", method: "GET", headers: map[string]string{"If-None-Match": etag}, status: 304},
						{name: "head304", method: "HEAD", headers: map[string]string{"If-None-Match": etag}, status: 304},
						{name: "other variant ETag", method: "GET", headers: map[string]string{"If-None-Match": other}, status: 200, body: want},
						{name: "range", method: "GET", headers: map[string]string{"Range": "bytes=0-9"}, status: 206, body: want[:10]},
						{name: "if range matches", method: "GET", headers: map[string]string{"Range": "bytes=0-9", "If-Range": etag}, status: 206, body: want[:10]},
						{name: "if range stale", method: "GET", headers: map[string]string{"Range": "bytes=0-9", "If-Range": other}, status: 200, body: want},
					} {
						t.Run(tc.name, func(t *testing.T) {
							req := httptest.NewRequest(tc.method, url, nil)
							for k, v := range tc.headers {
								req.Header.Set(k, v)
							}
							rec := httptest.NewRecorder()
							router.ServeHTTP(rec, req)
							if rec.Code != tc.status || !bytes.Equal(rec.Body.Bytes(), tc.body) {
								t.Fatalf("status=%d bytes=%d want status=%d bytes=%d", rec.Code, rec.Body.Len(), tc.status, len(tc.body))
							}
							if rec.Header().Get("ETag") != etag || rec.Header().Get("Cache-Control") != "public, max-age=86400" {
								t.Fatal("wrong cache headers", rec.Header())
							}
							if tc.method == "HEAD" && tc.status == 200 && rec.Header().Get("Content-Length") != strconv.Itoa(len(want)) {
								t.Fatal("HEAD length does not describe GET")
							}
						})
					}
					for _, badRange := range []string{"bytes=999999999-", "bytes=invalid"} {
						req := httptest.NewRequest("GET", url, nil)
						req.Header.Set("Range", badRange)
						rec := httptest.NewRecorder()
						router.ServeHTTP(rec, req)
						if rec.Code != 416 || rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("ETag") != "" {
							t.Fatalf("bad range %q: status=%d Cache-Control=%q ETag=%q", badRange, rec.Code, rec.Header().Get("Cache-Control"), rec.Header().Get("ETag"))
						}
					}
				})
			}
			for _, tc := range []struct {
				path   string
				status int
			}{
				{"/photos/invalid", 400},
				{"/photos/" + uuid.NewString(), 404},
				{"/photos/" + photoID.String() + "?variant=unknown", 400},
			} {
				rec := getPublic(router, tc.path)
				if rec.Code != tc.status || rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("ETag") != "" {
					t.Fatalf("error %s: status=%d Cache-Control=%q ETag=%q", tc.path, rec.Code, rec.Header().Get("Cache-Control"), rec.Header().Get("ETag"))
				}
			}
		})
	}
	bad := legacy
	bad.photo.DataURI = "data:image/jpeg;base64,aGVsbG8="
	router, _ := newAuthenticatedTestRouter(t, bad)
	rec := getPublic(router, "/photos/"+photoID.String())
	if rec.Code != 500 || rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("ETag") != "" {
		t.Fatal("invalid stored image must fail uncached")
	}
	failed := legacy
	failed.err = fmt.Errorf("fixture storage failure")
	router, _ = newAuthenticatedTestRouter(t, failed)
	rec = getPublic(router, "/photos/"+photoID.String())
	if rec.Code != 500 || rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("ETag") != "" {
		t.Fatal("backend failure must explicitly forbid storage")
	}
	if err = store.DeleteVisit(ctx, *visitID); err != nil {
		t.Fatal(err)
	}
	router, _ = newAuthenticatedTestRouter(t, store)
	for _, suffix := range []string{"", "?variant=thumb-v1"} {
		rec := getPublic(router, "/photos/"+photoID.String()+suffix)
		if rec.Code != 404 || rec.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("deleted image must be uncached404")
		}
	}
}
