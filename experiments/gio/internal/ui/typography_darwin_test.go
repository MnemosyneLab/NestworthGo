package ui

import (
	"slices"
	"testing"

	"gioui.org/font"
	"gioui.org/io/system"
	"gioui.org/text"
	"golang.org/x/image/math/fixed"
)

func TestMacThemeMixedScriptFontFallback(t *testing.T) {
	theme := NewTheme()
	for _, language := range []string{"en", "zh-CN"} {
		for _, weight := range []font.Weight{font.Normal, font.Medium, font.Bold} {
			glyphs := func(face font.Typeface, value string) []text.GlyphID {
				theme.Shaper.LayoutString(text.Parameters{
					Font: font.Font{Typeface: face, Weight: weight}, Locale: system.Locale{Language: language},
					PxPerEm: fixed.I(24), MaxWidth: 1024,
				}, value)
				var ids []text.GlyphID
				for g, ok := theme.Shaper.NextGlyph(); ok; g, ok = theme.Shaper.NextGlyph() {
					if g.ID != 0 {
						ids = append(ids, g.ID)
					}
				}
				return ids
			}
			for _, sample := range []struct {
				value    string
				fallback font.Typeface
			}{
				{"现", "Hiragino Sans GB"},
				{"💵", "Apple Color Emoji"},
				{"👩🏽‍💻", "Apple Color Emoji"},
			} {
				want := glyphs(sample.fallback, sample.value)
				if len(want) != 1 {
					t.Fatalf("expected one glyph for %q in %s, got %v", sample.value, sample.fallback, want)
				}
				for _, face := range []font.Typeface{theme.Face, systemTypeface(serifTypeface)} {
					if got := glyphs(face, sample.value); !slices.Equal(got, want) {
						t.Errorf("%s %s weight %d %q: got %v, want %s glyph %v", language, face, weight, sample.value, got, sample.fallback, want)
					}
				}
			}
			const latinAmount = "Café · US$25.00"
			if want, got := glyphs("Avenir Next, Helvetica Neue", latinAmount), glyphs(theme.Face, latinAmount); !slices.Equal(got, want) {
				t.Errorf("%s weight %d: the fallback changed Latin text or figures", language, weight)
			}
		}
	}
}
