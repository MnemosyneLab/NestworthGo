package media

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestNormalizeResizesAndStoresPNG(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 1024, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 1024; x++ {
			source.Set(x, y, color.RGBA{R: 20, G: 80, B: 120, A: 255})
		}
	}
	var input bytes.Buffer
	if err := png.Encode(&input, source); err != nil {
		t.Fatalf("encode source: %v", err)
	}
	output, err := Normalize(input.Bytes())
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	decoded, format, err := image.Decode(bytes.NewReader(output))
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if format != "png" || decoded.Bounds().Dx() != MaxDimension || decoded.Bounds().Dy() != 128 {
		t.Fatalf("output format=%s size=%dx%d", format, decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}

func TestNormalizeRejectsOversizedInput(t *testing.T) {
	if _, err := Normalize(bytes.Repeat([]byte{1}, MaxInputBytes+1)); err == nil {
		t.Fatal("oversized input succeeded")
	}
}
