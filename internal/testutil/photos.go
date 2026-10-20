// Package testutil provides original, procedurally generated test imagery.
// The pixel patterns are created here; no external photographs or rights are involved.
package testutil

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
)

func JPEG(width, height int, seed byte) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			img.SetRGBA(x, y, color.RGBA{byte(x*137/max(1, width)) + seed, byte(y * 173 / max(1, height)), byte((x + y) * 71 / max(1, width+height)), 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 85}); err != nil {
		panic(err)
	}
	return b.Bytes()
}

func PhotoURI(seed byte) string { return URI(JPEG(8, 6, seed)) }
func URI(data []byte) string {
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)
}
