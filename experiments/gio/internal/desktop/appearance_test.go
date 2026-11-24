package desktop

import (
	"image/color"
	"math"
	"testing"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

func TestThemeContrast(t *testing.T) {
	for _, theme := range accentThemes {
		for _, mode := range []struct {
			name   string
			colors palette
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
					{"page", c.pageBackground}, {"sidebar", c.sidebarBackground},
					{"field and card", c.surface}, {"hovered option", c.hoverSurface},
				} {
					bg := relativeLuminance(background.color)
					check("primary text on "+background.name, relativeLuminance(c.ink), bg, 4.5)
					check("secondary text on "+background.name, relativeLuminance(c.secondaryInk), bg, 4.5)
				}
				accent, surface := relativeLuminance(c.accent), relativeLuminance(c.surface)
				check("selected option text", relativeLuminance(c.onAccent), accent, 4.5)
				check("focused control", accent, surface, 3)
				check("error text on page", relativeLuminance(c.errorInk), relativeLuminance(c.pageBackground), 4.5)
				check("error text on card", relativeLuminance(c.errorInk), surface, 4.5)

				selection := c.editorSelection()
				opacity := float64(selection.A) / 255
				selected := relativeLuminance(selection)*opacity + surface*(1-opacity)
				check("selected editor text", relativeLuminance(c.ink), selected, 4.5)
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
