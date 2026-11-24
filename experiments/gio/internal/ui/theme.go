package ui

import (
	"image/color"

	"gioui.org/widget"
)

type Palette struct {
	PageBackground    color.NRGBA
	SidebarBackground color.NRGBA
	Surface           color.NRGBA
	HoverSurface      color.NRGBA
	Divider           color.NRGBA
	Ink               color.NRGBA
	SecondaryInk      color.NRGBA
	Accent            color.NRGBA
	OnAccent          color.NRGBA
	ErrorInk          color.NRGBA
}

func (p Palette) EditorSelection() color.NRGBA {
	selection := p.Accent
	// Keep the editor's text readable on the bright accents in dark mode.
	selection.A = 0x48
	return selection
}

func MustIcon(iconData []byte) *widget.Icon {
	icon, err := widget.NewIcon(iconData)
	if err != nil {
		panic(err)
	}
	return icon
}
