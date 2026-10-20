package model

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image/jpeg"
	"strings"
	"testing"

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
