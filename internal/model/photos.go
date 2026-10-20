package model

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
)

const (
	PhotoThumbnailVersion = "thumb-v1"
	PhotoThumbnailSize    = 320
	MaxPhotoDimension     = 8192
	MaxPhotoPixels        = 16_000_000
)

// Bound decoded-image memory across uploads, legacy reads, and backfill workers.
var photoDecoders = make(chan struct{}, 2)

// DecodeVisitPhotoDataURI checks compressed size and dimensions before allocating
// a decoded image. It validates the full JPEG but preserves its exact input bytes.
func DecodeVisitPhotoDataURI(raw string) ([]byte, error) {
	photoDecoders <- struct{}{}
	defer func() { <-photoDecoders }()
	data, err := photoURIBytes(raw)
	if err != nil {
		return nil, err
	}
	_, err = decodePhoto(data)
	return data, err
}

func photoURIBytes(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("photo data is required")
	}
	if !strings.HasPrefix(strings.ToLower(raw), visitPhotoPrefix) {
		return nil, errors.New("dine photos must be JPEG images")
	}
	encoded := raw[len(visitPhotoPrefix):]
	if len(encoded) > base64.StdEncoding.EncodedLen(MaxVisitPhotoBytes) {
		return nil, fmt.Errorf("dine photos must be smaller than %dKB", MaxVisitPhotoBytes/1024)
	}
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return nil, errors.New("dine photos must contain valid base64 image data")
	}
	if len(data) > MaxVisitPhotoBytes {
		return nil, fmt.Errorf("dine photos must be smaller than %dKB", MaxVisitPhotoBytes/1024)
	}
	return data, nil
}

func decodePhoto(data []byte) (image.Image, error) {
	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid JPEG: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > MaxPhotoDimension || config.Height > MaxPhotoDimension || config.Width > MaxPhotoPixels/config.Height {
		return nil, errors.New("dine photos exceed the decoded image size limit")
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("invalid JPEG: %w", err)
	}
	return img, nil
}

// PrepareVisitPhoto creates the versioned, aspect-preserving thumbnail. The
// original is never recompressed. Integer area averaging gives a bounded,
// deterministic downsample without an additional image-processing dependency.
func PrepareVisitPhoto(ctx context.Context, raw string) (original, thumbnail []byte, err error) {
	select {
	case photoDecoders <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	defer func() { <-photoDecoders }()
	original, err = photoURIBytes(raw)
	if err != nil {
		return nil, nil, err
	}
	img, err := decodePhoto(original)
	if err != nil {
		return nil, nil, err
	}
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	tw, th := width, height
	if width > PhotoThumbnailSize || height > PhotoThumbnailSize {
		if width >= height {
			tw = PhotoThumbnailSize
			th = max(1, height*tw/width)
		} else {
			th = PhotoThumbnailSize
			tw = max(1, width*th/height)
		}
	}
	thumb := image.NewRGBA(image.Rect(0, 0, tw, th))
	for y := range th {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		y0, y1 := y*height/th, (y+1)*height/th
		for x := range tw {
			x0, x1 := x*width/tw, (x+1)*width/tw
			var red, green, blue uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					r, g, b, _ := img.At(bounds.Min.X+sx, bounds.Min.Y+sy).RGBA()
					red += uint64(r)
					green += uint64(g)
					blue += uint64(b)
				}
			}
			count := uint64((x1 - x0) * (y1 - y0))
			thumb.SetRGBA(x, y, color.RGBA{uint8(red / count >> 8), uint8(green / count >> 8), uint8(blue / count >> 8), 255})
		}
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, thumb, &jpeg.Options{Quality: 80}); err != nil {
		return nil, nil, err
	}
	return original, encoded.Bytes(), nil
}
