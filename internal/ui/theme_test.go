package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	fyneTheme "fyne.io/fyne/v2/theme"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestPaletteForProvidesDistinctAccents(t *testing.T) {
	accents := []settings.Accent{
		settings.AccentNestworth,
		settings.AccentOcean,
		settings.AccentForest,
		settings.AccentAmber,
		settings.AccentRose,
	}
	seen := make(map[string]settings.Accent)
	for _, accent := range accents {
		palette := PaletteFor(accent)
		key := fmtColor(palette.Primary)
		if previous, ok := seen[key]; ok {
			t.Fatalf("accent %q reuses primary color from %q", accent, previous)
		}
		seen[key] = accent
	}
}

func TestThemeHonorsAccentAndAppearanceMode(t *testing.T) {
	pref := settings.Default()
	pref.Accent = settings.AccentRose
	pref.Appearance = settings.AppearanceDark
	theme := NewTheme(pref)
	if got := fmtColor(theme.Color(fyneTheme.ColorNamePrimary, fyneTheme.VariantLight)); got != fmtColor(PaletteFor(settings.AccentRose).Primary) {
		t.Fatalf("primary color = %s, want rose", got)
	}
	base := fyneTheme.DefaultTheme()
	if got, want := fmtColor(theme.Color(fyneTheme.ColorNameBackground, fyneTheme.VariantLight)), fmtColor(base.Color(fyneTheme.ColorNameBackground, fyneTheme.VariantDark)); got != want {
		t.Fatalf("dark background = %s, want %s", got, want)
	}
}

func fmtColor(value interface{ RGBA() (r, g, b, a uint32) }) string {
	r, g, b, a := value.RGBA()
	return string([]byte{byte(r >> 8), byte(g >> 8), byte(b >> 8), byte(a >> 8)})
}

var _ fyne.Theme = NewTheme(settings.Default())
