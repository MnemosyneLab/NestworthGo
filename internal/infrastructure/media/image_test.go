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

func TestFitRoundsScaledEdgeInsteadOfTruncating(t *testing.T) {
	// 49*MaxDimension/513 = 48.90…; truncation yields 48 and skews the
	// aspect ratio by a full pixel.
	fitted := fit(image.NewRGBA(image.Rect(0, 0, 513, 49)), MaxDimension)
	if width := fitted.Bounds().Dx(); width != MaxDimension {
		t.Fatalf("width = %d, want %d", width, MaxDimension)
	}
	if height := fitted.Bounds().Dy(); height != 49 {
		t.Fatalf("height = %d, want 49", height)
	}
}

func TestFitRoundsPortraitScaledEdgeInsteadOfTruncating(t *testing.T) {
	fitted := fit(image.NewRGBA(image.Rect(0, 0, 49, 513)), MaxDimension)
	if width := fitted.Bounds().Dx(); width != 49 {
		t.Fatalf("width = %d, want 49", width)
	}
	if height := fitted.Bounds().Dy(); height != MaxDimension {
		t.Fatalf("height = %d, want %d", height, MaxDimension)
	}
}

func TestFitKeepsAspectRatioOnRoundedEdges(t *testing.T) {
	fitted := fit(image.NewRGBA(image.Rect(0, 0, 1000, 333)), MaxDimension)
	if got := fitted.Bounds().Dx(); got != MaxDimension {
		t.Fatalf("width = %d, want %d", got, MaxDimension)
	}
	if got := fitted.Bounds().Dy(); got != 170 {
		t.Fatalf("height = %d, want 170 (rounded from 170.496)", got)
	}
}
