package main

import (
	"image/color"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

// The gallery copies the default Indigo surface tokens from the desktop shell
// (internal/desktop/appearance.go), because the shell's palettes are not
// exported. A page that adopts the charts reads its palette from its own
// ui.View and needs none of this.
func rgb(hex uint32) color.NRGBA {
	return color.NRGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 255}
}

var lightPalette = ui.Palette{
	PageBackground:    rgb(0xf9f7f3),
	SidebarBackground: rgb(0xf5f2ee),
	Surface:           rgb(0xfefdfc),
	HoverSurface:      rgb(0xf2efea),
	Divider:           rgb(0xe2dfda),
	Ink:               rgb(0x1f1a15),
	SecondaryInk:      rgb(0x625d56),
	Accent:            rgb(0x324ca7),
	OnAccent:          rgb(0xf9fafe),
	ErrorInk:          rgb(0xa4242a),
}.WithSemantics(false)

var darkPalette = ui.Palette{
	PageBackground:    rgb(0x100e0c),
	SidebarBackground: rgb(0x1a1815),
	Surface:           rgb(0x191714),
	HoverSurface:      rgb(0x23201c),
	Divider:           rgb(0x302c28),
	Ink:               rgb(0xedebe7),
	SecondaryInk:      rgb(0xa5a19b),
	Accent:            rgb(0x99b1fb),
	OnAccent:          rgb(0x0a0f1c),
	ErrorInk:          rgb(0xff9c9d),
}.WithSemantics(true)

func palette(dark bool) *ui.Palette {
	if dark {
		return &darkPalette
	}
	return &lightPalette
}
