package desktop

import (
	"image/color"
	"math"
	"testing"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func TestThemeContrast(t *testing.T) {
	for _, theme := range accentThemes {
		for _, mode := range []struct {
			name   string
			colors ui.Palette
		}{
			{"light", theme.light},
			{"dark", theme.dark},
		} {
			t.Run(i18n.English.Text(theme.label)+"/"+mode.name, func(t *testing.T) {
				c := mode.colors
				check := func(label string, fg, bg float64, minimum float64) {
					t.Helper()
					ratio := (max(fg, bg) + 0.05) / (min(fg, bg) + 0.05)
					if ratio < minimum {
						t.Errorf("%s contrast is %.2f:1, want at least %.1f:1", label, ratio, minimum)
					}
				}
				for _, background := range []struct {
					name  string
					color color.NRGBA
				}{
					{"page", c.PageBackground}, {"sidebar", c.SidebarBackground},
					{"field and card", c.Surface}, {"hovered option", c.HoverSurface},
				} {
					bg := relativeLuminance(background.color)
					check("primary text on "+background.name, relativeLuminance(c.Ink), bg, 4.5)
					check("secondary text on "+background.name, relativeLuminance(c.SecondaryInk), bg, 4.5)
				}
				accent, surface := relativeLuminance(c.Accent), relativeLuminance(c.Surface)
				check("selected option text", relativeLuminance(c.OnAccent), accent, 4.5)
				check("focused control", accent, surface, 3)
				check("error text on page", relativeLuminance(c.ErrorInk), relativeLuminance(c.PageBackground), 4.5)
				check("error text on card", relativeLuminance(c.ErrorInk), surface, 4.5)

				selection := c.EditorSelection()
				opacity := float64(selection.A) / 255
				selected := relativeLuminance(selection)*opacity + surface*(1-opacity)
				check("selected editor text", relativeLuminance(c.Ink), selected, 4.5)
			})
		}
	}
}

func relativeLuminance(c color.NRGBA) float64 {
	linear := func(channel uint8) float64 {
		s := float64(channel) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
}
