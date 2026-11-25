package desktop

import (
	"fmt"
	"testing"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func TestPalettesMeetContrastAndShareStatusTokens(t *testing.T) {
	for _, theme := range accentThemes {
		for _, dark := range []bool{false, true} {
			p := theme.light
			if dark {
				p = theme.dark
			}
			name := fmt.Sprintf("swatch #%02x%02x%02x dark=%v", theme.swatch.R, theme.swatch.G, theme.swatch.B, dark)
			if p.Dark != dark {
				t.Errorf("%s: palette Dark flag = %v", name, p.Dark)
			}
			for _, check := range []struct {
				what    string
				minimum float64
				ratioOf func() float64
			}{
				{what: "ink on page", minimum: 7, ratioOf: func() float64 { return ui.Contrast(p.Ink, p.PageBackground) }},
				{what: "ink on surface", minimum: 7, ratioOf: func() float64 { return ui.Contrast(p.Ink, p.Surface) }},
				{what: "secondary ink on surface", minimum: 4.5, ratioOf: func() float64 { return ui.Contrast(p.SecondaryInk, p.Surface) }},
				{what: "secondary ink on page", minimum: 4.5, ratioOf: func() float64 { return ui.Contrast(p.SecondaryInk, p.PageBackground) }},
				{what: "secondary ink on sidebar", minimum: 4.5, ratioOf: func() float64 { return ui.Contrast(p.SecondaryInk, p.SidebarBackground) }},
				{what: "secondary ink on hover", minimum: 4.5, ratioOf: func() float64 { return ui.Contrast(p.SecondaryInk, p.HoverSurface) }},
				{what: "on-accent on accent", minimum: 4.5, ratioOf: func() float64 { return ui.Contrast(p.OnAccent, p.Accent) }},
				{what: "accent on surface", minimum: 3, ratioOf: func() float64 { return ui.Contrast(p.Accent, p.Surface) }},
				{what: "error ink on surface", minimum: 4.5, ratioOf: func() float64 { return ui.Contrast(p.ErrorInk, p.Surface) }},
				{what: "divider on surface (non-text)", minimum: 1.1, ratioOf: func() float64 { return ui.Contrast(p.Divider, p.Surface) }},
			} {
				if got := check.ratioOf(); got < check.minimum {
					t.Errorf("%s: %s contrast %.2f is below %.1f", name, check.what, got, check.minimum)
				}
			}
			// Gain, loss and provenance must not vary with the accent.
			base := lightPalette
			if dark {
				base = darkPalette
			}
			if p.Success != base.Success || p.Destructive != base.Destructive || p.Provenance != base.Provenance ||
				p.Category != base.Category || p.ShadowTint != base.ShadowTint {
				t.Errorf("%s: status tokens differ from the shared palette", name)
			}
			if p.Success.A == 0 || p.Destructive.A == 0 || p.Category[0].A == 0 {
				t.Errorf("%s: semantic tokens were not filled", name)
			}
		}
	}
}
