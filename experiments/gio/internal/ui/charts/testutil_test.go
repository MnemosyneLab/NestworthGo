package charts

import (
	"image/color"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func testPalette(dark bool) *ui.Palette {
	p := ui.Palette{Surface: rgbaOf(dark, 0x191714, 0xfefdfc), Ink: rgbaOf(dark, 0xedebe7, 0x1f1a15)}.WithSemantics(dark)
	return &p
}

func day(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func rgbaOf(dark bool, darkHex, lightHex uint32) (c color.NRGBA) {
	h := lightHex
	if dark {
		h = darkHex
	}
	return color.NRGBA{R: uint8(h >> 16), G: uint8(h >> 8), B: uint8(h), A: 255}
}
