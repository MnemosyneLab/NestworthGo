package ui

import (
	"image/color"
	"testing"
)

func TestOKLCHMatchesWailsSurfaceTokens(t *testing.T) {
	// Reference values are the sRGB colors the Gio palettes already ship for
	// these index.css tokens, so a drift in the converter shows up here.
	for _, sample := range []struct {
		name    string
		l, c, h float64
		want    color.NRGBA
	}{
		{"primary light", 0.45, 0.15, 268, color.NRGBA{0x32, 0x4c, 0xa7, 0xff}},
		{"primary dark", 0.77, 0.11, 270, color.NRGBA{0x99, 0xb1, 0xfb, 0xff}},
		{"background light", 0.976, 0.005, 85, color.NRGBA{0xf9, 0xf7, 0xf3, 0xff}},
		{"card light", 0.995, 0.002, 85, color.NRGBA{0xfe, 0xfd, 0xfc, 0xff}},
		{"foreground light", 0.22, 0.012, 65, color.NRGBA{0x1f, 0x1a, 0x15, 0xff}},
		{"border light", 0.905, 0.008, 80, color.NRGBA{0xe2, 0xdf, 0xda, 0xff}},
		{"muted foreground light", 0.48, 0.012, 70, color.NRGBA{0x62, 0x5d, 0x56, 0xff}},
		{"muted light", 0.952, 0.007, 80, color.NRGBA{0xf2, 0xef, 0xea, 0xff}},
		{"background dark", 0.165, 0.006, 70, color.NRGBA{0x10, 0x0e, 0x0c, 0xff}},
	} {
		if got := OKLCH(sample.l, sample.c, sample.h); got != sample.want {
			t.Errorf("%s: got %#v, want %#v", sample.name, got, sample.want)
		}
	}
}

func TestMixAndAlpha(t *testing.T) {
	white, black := color.NRGBA{255, 255, 255, 255}, color.NRGBA{0, 0, 0, 255}
	if got := Mix(white, black, 0.5); got != (color.NRGBA{128, 128, 128, 255}) {
		t.Fatalf("half mix = %#v", got)
	}
	if Mix(white, black, 0) != white || Mix(white, black, 1) != black || Mix(white, black, 7) != black {
		t.Fatal("mix did not clamp its fraction")
	}
	if got := WithAlpha(black, 0.5); got.A != 128 || got.R != 0 {
		t.Fatalf("alpha = %#v", got)
	}
	if got := Contrast(white, black); got < 20.9 || got > 21.1 {
		t.Fatalf("black on white contrast = %v, want 21", got)
	}
}
