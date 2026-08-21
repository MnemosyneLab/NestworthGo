package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	fyneTheme "fyne.io/fyne/v2/theme"
	"github.com/waltwang/nestworth-go/internal/settings"
)

// AccentPalette keeps the application accent coherent across custom cards,
// navigation, progress bars, and Fyne's built-in primary controls.
type AccentPalette struct {
	Name      settings.Accent
	Primary   color.NRGBA
	OnPrimary color.NRGBA
	Soft      color.NRGBA
	Selection color.NRGBA
}

func PaletteFor(accent settings.Accent) AccentPalette {
	switch accent {
	case settings.AccentOcean:
		return palette(accent, "#2D74D6", "#FFFFFF", "#EAF2FF", "#BBD3FF")
	case settings.AccentForest:
		return palette(accent, "#2B865A", "#FFFFFF", "#EAF7EF", "#BFE4CD")
	case settings.AccentAmber:
		return palette(accent, "#A8650A", "#FFFFFF", "#FFF4DE", "#F4D39A")
	case settings.AccentRose:
		return palette(accent, "#B84263", "#FFFFFF", "#FFF0F3", "#F2BBC8")
	default:
		return palette(settings.AccentNestworth, "#079C9A", "#FFFFFF", "#E6F8F7", "#A9E5E1")
	}
}

func palette(name settings.Accent, primary, onPrimary, soft, selection string) AccentPalette {
	return AccentPalette{
		Name:      name,
		Primary:   parseHex(primary),
		OnPrimary: parseHex(onPrimary),
		Soft:      parseHex(soft),
		Selection: parseHex(selection),
	}
}

func parseHex(value string) color.NRGBA {
	if len(value) != 7 || value[0] != '#' {
		return color.NRGBA{A: 0xFF}
	}
	var result color.NRGBA
	_, _ = fmtHex(value[1:3], &result.R)
	_, _ = fmtHex(value[3:5], &result.G)
	_, _ = fmtHex(value[5:7], &result.B)
	result.A = 0xFF
	return result
}

func fmtHex(value string, target *uint8) (int, error) {
	var result uint8
	for _, r := range value {
		result <<= 4
		switch {
		case r >= '0' && r <= '9':
			result += uint8(r - '0')
		case r >= 'a' && r <= 'f':
			result += uint8(r-'a') + 10
		case r >= 'A' && r <= 'F':
			result += uint8(r-'A') + 10
		default:
			return 0, nil
		}
	}
	*target = result
	return 1, nil
}

type nestworthTheme struct {
	base    fyne.Theme
	palette AccentPalette
	mode    settings.Appearance
}

func NewTheme(preference settings.Settings) fyne.Theme {
	return &nestworthTheme{
		base:    fyneTheme.DefaultTheme(),
		palette: PaletteFor(preference.Accent),
		mode:    preference.Appearance,
	}
}

func ApplyTheme(application fyne.App, preference settings.Settings) {
	if application == nil {
		return
	}
	application.Settings().SetTheme(NewTheme(preference))
}

func (t *nestworthTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case fyneTheme.ColorNamePrimary, fyneTheme.ColorNameHyperlink:
		return t.palette.Primary
	case fyneTheme.ColorNameForegroundOnPrimary:
		return t.palette.OnPrimary
	case fyneTheme.ColorNameFocus:
		return t.palette.Primary
	case fyneTheme.ColorNameSelection:
		return t.palette.Selection
	}
	if t.mode == settings.AppearanceLight {
		variant = fyneTheme.VariantLight
	} else if t.mode == settings.AppearanceDark {
		variant = fyneTheme.VariantDark
	}
	return t.base.Color(name, variant)
}

func (t *nestworthTheme) Font(style fyne.TextStyle) fyne.Resource {
	return t.base.Font(style)
}

func (t *nestworthTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.base.Icon(name)
}

func (t *nestworthTheme) Size(name fyne.ThemeSizeName) float32 {
	return t.base.Size(name)
}

func currentColor(name fyne.ThemeColorName) color.Color {
	if fyne.CurrentApp() == nil {
		return color.NRGBA{A: 0xFF}
	}
	return fyneTheme.Current().Color(name, fyne.CurrentApp().Settings().ThemeVariant())
}
