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
	onAccent          color.NRGBA
	errorInk          color.NRGBA
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
	onAccent:          color.NRGBA{R: 248, G: 246, B: 243, A: 255},
	errorInk:          color.NRGBA{R: 164, G: 36, B: 42, A: 255},
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
	onAccent:          color.NRGBA{R: 23, G: 26, B: 32, A: 255},
	errorInk:          color.NRGBA{R: 255, G: 156, B: 157, A: 255},
}

type accentTheme struct {
	label       messageID
	light, dark palette
}

// The non-default themes map the Wails OKLCH surface and primary tokens in
// frontend/src/index.css to sRGB. Error colors stay independent of the accent.
var accentThemes = [...]accentTheme{
	{label: indigoAccent, light: lightPalette, dark: darkPalette},
	{
		label: lavenderAccent,
		light: palette{
			pageBackground:    rgb(0xf9f6fe),
			sidebarBackground: rgb(0xf5f2fc),
			surface:           rgb(0xfefdfc),
			hoverSurface:      rgb(0xf2effa),
			divider:           rgb(0xe5e0ef),
			ink:               rgb(0x201c2c),
			secondaryInk:      rgb(0x5f5a70),
			accent:            rgb(0x7550c2),
			onAccent:          rgb(0xfcfbff),
			errorInk:          lightPalette.errorInk,
		},
		dark: palette{
			pageBackground:    rgb(0x12101a),
			sidebarBackground: rgb(0x1c1926),
			surface:           rgb(0x1b1826),
			hoverSurface:      rgb(0x252130),
			divider:           rgb(0x342f43),
			ink:               rgb(0xefedf4),
			secondaryInk:      rgb(0xa49eaf),
			accent:            rgb(0xbd9ff2),
			onAccent:          rgb(0x100d1b),
			errorInk:          darkPalette.errorInk,
		},
	},
	{
		label: mintAccent,
		light: palette{
			pageBackground:    rgb(0xeffaf7),
			sidebarBackground: rgb(0xeaf6f4),
			surface:           rgb(0xfefdfc),
			hoverSurface:      rgb(0xe6f3f1),
			divider:           rgb(0xd3e6e3),
			ink:               rgb(0x0b2425),
			secondaryInk:      rgb(0x476061),
			accent:            rgb(0x007e6c),
			onAccent:          rgb(0xf5fefc),
			errorInk:          lightPalette.errorInk,
		},
		dark: palette{
			pageBackground:    rgb(0x061415),
			sidebarBackground: rgb(0x0f1f1f),
			surface:           rgb(0x0c1e1f),
			hoverSurface:      rgb(0x172728),
			divider:           rgb(0x213435),
			ink:               rgb(0xe7f1f0),
			secondaryInk:      rgb(0x8e9f9f),
			accent:            rgb(0x30ccb1),
			onAccent:          rgb(0x001411),
			errorInk:          darkPalette.errorInk,
		},
	},
	{
		label: skyAccent,
		light: palette{
			pageBackground:    rgb(0xf0f9fe),
			sidebarBackground: rgb(0xebf5fc),
			surface:           rgb(0xfefdfc),
			hoverSurface:      rgb(0xe7f2fa),
			divider:           rgb(0xd2e2ee),
			ink:               rgb(0x101e2d),
			secondaryInk:      rgb(0x495a6b),
			accent:            rgb(0x006cce),
			onAccent:          rgb(0xf9fcff),
			errorInk:          lightPalette.errorInk,
		},
		dark: palette{
			pageBackground:    rgb(0x07111d),
			sidebarBackground: rgb(0x111c2a),
			surface:           rgb(0x0f1b2b),
			hoverSurface:      rgb(0x192534),
			divider:           rgb(0x223144),
			ink:               rgb(0xe9f0f5),
			secondaryInk:      rgb(0x92a0ad),
			accent:            rgb(0x55b8f8),
			onAccent:          rgb(0x040e1a),
			errorInk:          darkPalette.errorInk,
		},
	},
	{
		label: peachAccent,
		light: palette{
			pageBackground:    rgb(0xfef7f0),
			sidebarBackground: rgb(0xfbf3ea),
			surface:           rgb(0xfefdfc),
			hoverSurface:      rgb(0xf9efe4),
			divider:           rgb(0xeeded0),
			ink:               rgb(0x2b1b13),
			secondaryInk:      rgb(0x6c5549),
			accent:            rgb(0xca430e),
			onAccent:          rgb(0xfffaf8),
			errorInk:          lightPalette.errorInk,
		},
		dark: palette{
			pageBackground:    rgb(0x190f0a),
			sidebarBackground: rgb(0x251812),
			surface:           rgb(0x251710),
			hoverSurface:      rgb(0x2e2019),
			divider:           rgb(0x402e24),
			ink:               rgb(0xf4ede6),
			secondaryInk:      rgb(0xaa9b90),
			accent:            rgb(0xf99262),
			onAccent:          rgb(0x1a0b05),
			errorInk:          darkPalette.errorInk,
		},
	},
}

func rgb(hex uint32) color.NRGBA {
	return color.NRGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 255}
}

func (p palette) editorSelection() color.NRGBA {
	selection := p.accent
	// Keep the editor's text readable on the bright accents in dark mode.
	selection.A = 0x48
	return selection
}

type sessionPreferences struct {
	language    appLanguage
	dark        bool
	accentIndex int
}

func (p sessionPreferences) colors() *palette {
	theme := &accentThemes[p.accentIndex]
	if p.dark {
		return &theme.dark
	}
	return &theme.light
}
