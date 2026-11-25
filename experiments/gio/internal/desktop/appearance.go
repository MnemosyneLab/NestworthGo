package desktop

import (
	"image/color"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

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
}

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
}

type accentTheme struct {
	label       i18n.MessageID
	swatch      color.NRGBA
	light, dark ui.Palette
}

// These palettes map the Wails OKLCH surface and primary tokens in
// frontend/src/index.css to sRGB. The swatches use the fixed preview colors
// in the Wails Settings page.
var accentThemes = [...]accentTheme{
	{label: i18n.IndigoAccent, swatch: rgb(0x4364df), light: lightPalette, dark: darkPalette},
	{
		label:  i18n.LavenderAccent,
		swatch: rgb(0x9b74d9),
		light: ui.Palette{
			PageBackground:    rgb(0xf9f6fe),
			SidebarBackground: rgb(0xf5f2fc),
			Surface:           rgb(0xfefdfc),
			HoverSurface:      rgb(0xf2effa),
			Divider:           rgb(0xe5e0ef),
			Ink:               rgb(0x201c2c),
			SecondaryInk:      rgb(0x5f5a70),
			Accent:            rgb(0x7550c2),
			OnAccent:          rgb(0xfcfbff),
			ErrorInk:          lightPalette.ErrorInk,
		},
		dark: ui.Palette{
			PageBackground:    rgb(0x12101a),
			SidebarBackground: rgb(0x1c1926),
			Surface:           rgb(0x1b1826),
			HoverSurface:      rgb(0x252130),
			Divider:           rgb(0x342f43),
			Ink:               rgb(0xefedf4),
			SecondaryInk:      rgb(0xa49eaf),
			Accent:            rgb(0xbd9ff2),
			OnAccent:          rgb(0x100d1b),
			ErrorInk:          darkPalette.ErrorInk,
		},
	},
	{
		label:  i18n.MintAccent,
		swatch: rgb(0x009783),
		light: ui.Palette{
			PageBackground:    rgb(0xeffaf7),
			SidebarBackground: rgb(0xeaf6f4),
			Surface:           rgb(0xfefdfc),
			HoverSurface:      rgb(0xe6f3f1),
			Divider:           rgb(0xd3e6e3),
			Ink:               rgb(0x0b2425),
			SecondaryInk:      rgb(0x476061),
			Accent:            rgb(0x007e6c),
			OnAccent:          rgb(0xf5fefc),
			ErrorInk:          lightPalette.ErrorInk,
		},
		dark: ui.Palette{
			PageBackground:    rgb(0x061415),
			SidebarBackground: rgb(0x0f1f1f),
			Surface:           rgb(0x0c1e1f),
			HoverSurface:      rgb(0x172728),
			Divider:           rgb(0x213435),
			Ink:               rgb(0xe7f1f0),
			SecondaryInk:      rgb(0x8e9f9f),
			Accent:            rgb(0x30ccb1),
			OnAccent:          rgb(0x001411),
			ErrorInk:          darkPalette.ErrorInk,
		},
	},
	{
		label:  i18n.SkyAccent,
		swatch: rgb(0x2389e2),
		light: ui.Palette{
			PageBackground:    rgb(0xf0f9fe),
			SidebarBackground: rgb(0xebf5fc),
			Surface:           rgb(0xfefdfc),
			HoverSurface:      rgb(0xe7f2fa),
			Divider:           rgb(0xd2e2ee),
			Ink:               rgb(0x101e2d),
			SecondaryInk:      rgb(0x495a6b),
			Accent:            rgb(0x006cce),
			OnAccent:          rgb(0xf9fcff),
			ErrorInk:          lightPalette.ErrorInk,
		},
		dark: ui.Palette{
			PageBackground:    rgb(0x07111d),
			SidebarBackground: rgb(0x111c2a),
			Surface:           rgb(0x0f1b2b),
			HoverSurface:      rgb(0x192534),
			Divider:           rgb(0x223144),
			Ink:               rgb(0xe9f0f5),
			SecondaryInk:      rgb(0x92a0ad),
			Accent:            rgb(0x55b8f8),
			OnAccent:          rgb(0x040e1a),
			ErrorInk:          darkPalette.ErrorInk,
		},
	},
	{
		label:  i18n.PeachAccent,
		swatch: rgb(0xf0834e),
		light: ui.Palette{
			PageBackground:    rgb(0xfef7f0),
			SidebarBackground: rgb(0xfbf3ea),
			Surface:           rgb(0xfefdfc),
			HoverSurface:      rgb(0xf9efe4),
			Divider:           rgb(0xeeded0),
			Ink:               rgb(0x2b1b13),
			SecondaryInk:      rgb(0x6c5549),
			Accent:            rgb(0xca430e),
			OnAccent:          rgb(0xfffaf8),
			ErrorInk:          lightPalette.ErrorInk,
		},
		dark: ui.Palette{
			PageBackground:    rgb(0x190f0a),
			SidebarBackground: rgb(0x251812),
			Surface:           rgb(0x251710),
			HoverSurface:      rgb(0x2e2019),
			Divider:           rgb(0x402e24),
			Ink:               rgb(0xf4ede6),
			SecondaryInk:      rgb(0xaa9b90),
			Accent:            rgb(0xf99262),
			OnAccent:          rgb(0x1a0b05),
			ErrorInk:          darkPalette.ErrorInk,
		},
	},
}

func init() {
	// Fill the status, provenance, category and shadow tokens that do not vary
	// by accent. accentThemes copied lightPalette and darkPalette at variable
	// initialization, so each theme is completed on its own.
	lightPalette, darkPalette = lightPalette.WithSemantics(false), darkPalette.WithSemantics(true)
	for i := range accentThemes {
		accentThemes[i].light = accentThemes[i].light.WithSemantics(false)
		accentThemes[i].dark = accentThemes[i].dark.WithSemantics(true)
	}
}

func rgb(hex uint32) color.NRGBA {
	return color.NRGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 255}
}

type sessionPreferences struct {
	language    i18n.Language
	dark        bool
	accentIndex int
}

func (p sessionPreferences) colors() *ui.Palette {
	theme := &accentThemes[p.accentIndex]
	if p.dark {
		return &theme.dark
	}
	return &theme.light
}
