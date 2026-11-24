package main

import "image/color"

type palette struct {
	pageBackground    color.NRGBA
	sidebarBackground color.NRGBA
	surface           color.NRGBA
	hoverSurface      color.NRGBA
	divider           color.NRGBA
	ink               color.NRGBA
	secondaryInk      color.NRGBA
	accent            color.NRGBA
}

var lightPalette = palette{
	pageBackground:    color.NRGBA{R: 248, G: 246, B: 243, A: 255},
	sidebarBackground: color.NRGBA{R: 242, G: 240, B: 236, A: 255},
	surface:           color.NRGBA{R: 254, G: 253, B: 252, A: 255},
	hoverSurface:      color.NRGBA{R: 234, G: 231, B: 226, A: 255},
	divider:           color.NRGBA{R: 223, G: 219, B: 212, A: 255},
	ink:               color.NRGBA{R: 32, G: 27, B: 23, A: 255},
	secondaryInk:      color.NRGBA{R: 95, G: 88, B: 82, A: 255},
	accent:            color.NRGBA{R: 52, G: 72, B: 166, A: 255},
}

var darkPalette = palette{
	pageBackground:    color.NRGBA{R: 23, G: 26, B: 32, A: 255},
	sidebarBackground: color.NRGBA{R: 29, G: 33, B: 41, A: 255},
	surface:           color.NRGBA{R: 35, G: 40, B: 50, A: 255},
	hoverSurface:      color.NRGBA{R: 43, G: 50, B: 62, A: 255},
	divider:           color.NRGBA{R: 60, G: 68, B: 82, A: 255},
	ink:               color.NRGBA{R: 237, G: 240, B: 246, A: 255},
	secondaryInk:      color.NRGBA{R: 180, G: 189, B: 205, A: 255},
	accent:            color.NRGBA{R: 160, G: 174, B: 255, A: 255},
}

type sessionPreferences struct {
	language appLanguage
	dark     bool
}

func (p sessionPreferences) colors() *palette {
	if p.dark {
		return &darkPalette
	}
	return &lightPalette
}
