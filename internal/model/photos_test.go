package model

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"github.com/google/uuid"
	"image/jpeg"
	"strings"
	"testing"
	"time"

	"github.com/bitofbytes-io/dined/internal/testutil"
)

func TestPrepareVisitPhotoPreservesOriginalAndBoundsThumbnail(t *testing.T) {
	for _, tc := range []struct{ w, h, tw, th int }{{1280, 960, 320, 240}, {600, 1200, 160, 320}, {8, 6, 8, 6}, {1, 400, 1, 320}} {
		t.Run(fmt.Sprintf("%dx%d", tc.w, tc.h), func(t *testing.T) {
			original := testutil.JPEG(tc.w, tc.h, 7)
			got, thumb, err := PrepareVisitPhoto(context.Background(), testutil.URI(original))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, original) {
				t.Fatal("original bytes changed")
			}
			cfg, err := jpeg.DecodeConfig(bytes.NewReader(thumb))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Width != tc.tw || cfg.Height != tc.th {
				t.Fatalf("thumbnail dimensions %dx%d want %dx%d", cfg.Width, cfg.Height, tc.tw, tc.th)
			}
			_, again, err := PrepareVisitPhoto(context.Background(), testutil.URI(original))
			if err != nil || !bytes.Equal(again, thumb) {
				t.Fatal("thumbnail is not deterministic", err)
			}
		})
	}
}

func TestVisitPhotoRejectsMalformedAndOversizedJPEG(t *testing.T) {
	valid := testutil.JPEG(8, 6, 1)
	oversized := append([]byte(nil), valid...)
	pos := bytes.Index(oversized, []byte{0xff, 0xc0})
	if pos < 0 {
		t.Fatal("missing SOF")
	}
	binary.BigEndian.PutUint16(oversized[pos+5:pos+7], 8192)
	binary.BigEndian.PutUint16(oversized[pos+7:pos+9], 8192)
	cases := map[string]string{
		"not a JPEG":             testutil.URI([]byte("hello")),
		"truncated":              testutil.URI(valid[:len(valid)/2]),
		"empty":                  testutil.URI(nil),
		"pixel bomb":             testutil.URI(oversized),
		"oversized encoded body": visitPhotoPrefix + strings.Repeat("A", MaxVisitPhotoBytes*2),
		"invalid base64":         visitPhotoPrefix + "%%%",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := PrepareVisitPhoto(context.Background(), raw); err == nil {
				t.Fatal("accepted invalid image")
			}
		})
	}
}

func TestPrepareVisitPhotoCancellation(t *testing.T) {
	// Occupy both slots to prove waiting for decode capacity honors cancellation.
	photoDecoders <- struct{}{}
	photoDecoders <- struct{}{}
	defer func() { <-photoDecoders; <-photoDecoders }()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := PrepareVisitPhoto(ctx, testutil.PhotoURI(1)); err != context.Canceled {
		t.Fatalf("error=%v", err)
	}
}

func TestPreparedVisitReusesValidatedPhotosWithoutDecoding(t *testing.T) {
	input := VisitInput{RestaurantName: "Prepared diner", VisitedAt: time.Now(), PriceLevel: 2, Ratings: map[uuid.UUID]float64{uuid.New(): 8}, Photos: []VisitPhotoInput{{DataURI: testutil.PhotoURI(1)}}}
	prepared, err := input.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if input.Photos[0].prepared != nil {
		t.Fatal("preparation mutated caller's input")
	}
	// Both decoder slots are busy: reuse must succeed without acquiring either.
	photoDecoders <- struct{}{}
	photoDecoders <- struct{}{}
	defer func() { <-photoDecoders; <-photoDecoders }()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	again, err := prepared.Prepare(ctx)
	if err != nil {
		t.Fatal("prepared photo was decoded again", err)
	}
	if again.Photos[0].prepared != prepared.Photos[0].prepared {
		t.Fatal("validated cache not reused")
	}
	photos, err := again.PreparedPhotos()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(photos[0].Bytes, testutil.JPEG(8, 6, 1)) {
		t.Fatal("original changed")
	}
	photos[0].Bytes[0] = 0
	photos[0].ThumbnailBytes[0] = 0
	untouched, err := again.PreparedPhotos()
	if err != nil {
		t.Fatal(err)
	}
	if untouched[0].Bytes[0] != 0xff || untouched[0].ThumbnailBytes[0] != 0xff {
		t.Fatal("store copies alias the private validation cache")
	}
}

func TestPreparedVisitRevalidatesChangedInput(t *testing.T) {
	input := VisitInput{RestaurantName: "Prepared diner", VisitedAt: time.Now(), PriceLevel: 2, Ratings: map[uuid.UUID]float64{uuid.New(): 8}, Photos: []VisitPhotoInput{{DataURI: testutil.PhotoURI(1)}}}
	prepared, err := input.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	prepared.PriceLevel = 0
	if _, err = prepared.Prepare(context.Background()); err == nil {
		t.Fatal("cached photo bypassed field validation")
	}
	prepared.PriceLevel = 2
	prepared.Photos[0].DataURI = "data:image/jpeg;base64,aGVsbG8="
	if _, err = prepared.PreparedPhotos(); err == nil {
		t.Fatal("changed URI reused old photo bytes")
	}
	if _, err = prepared.Prepare(context.Background()); err == nil {
		t.Fatal("changed invalid URI bypassed JPEG validation")
	}
	prepared.Photos[0].DataURI = testutil.PhotoURI(2)
	replaced, err := prepared.Prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	photos, err := replaced.PreparedPhotos()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(photos[0].Bytes, testutil.JPEG(8, 6, 2)) {
		t.Fatal("replacement kept stale original bytes")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = replaced.Prepare(ctx); err != context.Canceled {
		t.Fatal("cached preparation ignored cancellation", err)
	}
}
