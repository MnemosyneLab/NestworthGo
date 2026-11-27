package ui

import (
	"image"
	"math"
	"path/filepath"
	"slices"
	"testing"

	"gioui.org/font"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"golang.org/x/image/math/fixed"
)

func TestMacThemeMixedScriptFontFallback(t *testing.T) {
	theme := NewTheme()
	pingFang := installedMacPingFang(t)
	hasHangzhouNumeral := slices.ContainsFunc(pingFang, func(f font.FontFace) bool {
		glyph, _ := f.Face.Face().NominalGlyph('〸')
		return glyph != 0
	})
	for _, language := range []string{"en", "zh-CN"} {
		var pingFangWeights []text.GlyphID
		for _, weight := range []font.Weight{font.Normal, font.Medium, font.Bold} {
			shape := func(face font.Typeface, value string) []text.Glyph {
				return macGlyphs(theme.Shaper, text.Parameters{
					Font: font.Font{Typeface: face, Weight: weight}, Locale: system.Locale{Language: language},
					PxPerEm: fixed.I(24), MaxWidth: 1024,
				}, value)
			}
			assertFallback := func(value string, family font.Typeface) text.GlyphID {
				want := requireMacGlyph(t, shape(family, value), family, value)
				for _, face := range []font.Typeface{theme.Face, systemTypeface(serifTypeface)} {
					if got := requireMacGlyph(t, shape(face, value), face, value); got != want {
						t.Errorf("%s %s weight %d %q: got %v, want %s glyph %v", language, face, weight, value, got, family, want)
					}
				}
				return want
			}
			if len(pingFang) > 0 {
				want := assertFallback("现", pingFangSC)
				hiragino := requireMacGlyph(t, shape("Hiragino Sans GB", "现"), "Hiragino Sans GB", "现")
				if want == hiragino || slices.Contains(pingFangWeights, want) {
					t.Errorf("%s PingFang SC weight %d did not select a distinct face", language, weight)
				}
				pingFangWeights = append(pingFangWeights, want)
				if !hasHangzhouNumeral {
					assertFallback("〸", "Hiragino Sans GB")
				}
			}
			for _, value := range []string{"现", "繁體中文", "日本語かなカナ"} {
				for _, g := range shape(theme.Face, value) {
					if !macGlyphHasInk(g) {
						t.Errorf("%s weight %d %q has a missing glyph: %v", language, weight, value, g)
					}
				}
			}
			assertFallback("💵", "Apple Color Emoji")
			assertFallback("👩🏽‍💻", "Apple Color Emoji")
			const latinAmount = "Café · US$25.00"
			if want, got := shape(sansTypeface, latinAmount), shape(theme.Face, latinAmount); !slices.Equal(got, want) {
				t.Errorf("%s weight %d: the fallback changed Latin text or figures", language, weight)
			}
		}
	}
}

func TestMacPingFangControlLabelsAreCentered(t *testing.T) {
	t.Run("PingFang asset", func(t *testing.T) {
		if len(installedMacPingFang(t)) == 0 {
			t.Skip("the standard PingFang system font asset is not installed")
		}
		checkMacControlLabelAlignment(t, testView(), 1)
	})
	t.Run("without PingFang asset", func(t *testing.T) {
		view := testView()
		view.Theme.Face = macTypeface(sansTypeface, false)
		view.Theme.Shaper = text.NewShaper()
		checkMacControlLabelAlignment(t, view, 1.5)
	})
}

func checkMacControlLabelAlignment(t *testing.T, view View, maxOffsetDP float64) {
	t.Helper()
	button := new(widget.Clickable)
	for _, scale := range []float32{1, 2} {
		var healthBaseline int
		for _, sample := range []struct {
			label  string
			button bool
			tone   BadgeTone
		}{
			{label: "26 个活跃账户", tone: BadgeNeutral},
			{label: "完整", tone: BadgeSuccess},
			{label: "数据健康 · 25 项问题", button: true},
			{label: "数据健康 · 1 项问题", button: true},
			{label: "数据健康 · 1250 项问题", button: true},
			{label: "数据健康 · 25 项问题 (USD g25)", button: true},
		} {
			var ops op.Ops
			gtx := layout.Context{Ops: &ops, Constraints: layout.Constraints{Max: image.Pt(1024, 200)},
				Metric: unit.Metric{PxPerDp: scale, PxPerSp: scale}, Locale: system.Locale{Language: "zh-CN"}}
			style := TextCaptionStrong
			var control layout.Dimensions
			if sample.button {
				style = ButtonSmall.metrics().text
				control = view.ButtonOf(button, ButtonSpec{Variant: ButtonOutline, Size: ButtonSmall, Title: sample.label})(gtx)
				if control.Size.Y != gtx.Dp(ControlSmall) {
					t.Errorf("%.0f× %q: button height %d, want %d", scale, sample.label, control.Size.Y, gtx.Dp(ControlSmall))
				}
			} else {
				control = view.Badge(gtx, sample.tone, sample.label)
			}
			line := view.Line(gtx, style, sample.label, view.Colors.Ink)
			labelTop := (control.Size.Y - line.Size.Y) / 2
			glyphs := macGlyphs(view.Theme.Shaper, text.Parameters{
				Font:    font.Font{Typeface: view.Theme.Face, Weight: faceWeight(style.Weight, sample.label)},
				PxPerEm: fixed.I(gtx.Sp(style.Size)), MaxLines: 1, MaxWidth: gtx.Constraints.Max.X,
				Locale: gtx.Locale, LineHeightScale: style.LineHeight,
			}, sample.label)
			top, bottom := math.Inf(1), math.Inf(-1)
			for _, g := range glyphs {
				if g.Bounds.Min.Y == g.Bounds.Max.Y {
					continue // Spaces have no ink.
				}
				top = min(top, float64(g.Y)+float64(g.Bounds.Min.Y)/64)
				bottom = max(bottom, float64(g.Y)+float64(g.Bounds.Max.Y)/64)
			}
			offset := float64(labelTop) + (top+bottom-float64(control.Size.Y))/2
			if math.IsNaN(offset) || math.Abs(offset) > maxOffsetDP*float64(scale) {
				t.Errorf("%.0f× %q: ink center offset %+.2fpx", scale, sample.label, offset)
			}
			if sample.button {
				baseline := labelTop + line.Size.Y - line.Baseline
				if healthBaseline == 0 {
					healthBaseline = baseline
				}
				if baseline != healthBaseline {
					t.Errorf("%.0f× %q: baseline %d, want %d", scale, sample.label, baseline, healthBaseline)
				}
			}
		}
	}
}

func installedMacPingFang(t *testing.T) []font.FontFace {
	t.Helper()
	// Check for an installed standard collection independently of the loader,
	// so failing to discover or parse one cannot silently skip assertions.
	paths, err := filepath.Glob("/System/Library/AssetsV2/*/*/AssetData/PingFang.ttc")
	if err != nil {
		t.Fatal(err)
	}
	faces := supplementalSystemFaces()
	if len(paths) != 0 && len(faces) == 0 {
		t.Fatal("the installed PingFang system font asset yielded no usable faces")
	}
	return faces
}

func macGlyphs(shaper *text.Shaper, params text.Parameters, value string) []text.Glyph {
	shaper.LayoutString(params, value)
	var glyphs []text.Glyph
	for g, ok := shaper.NextGlyph(); ok; g, ok = shaper.NextGlyph() {
		glyphs = append(glyphs, g)
	}
	return glyphs
}

func macGlyphHasInk(g text.Glyph) bool {
	// Gio packs the font's glyph index into the low 32 bits. Index 0 is .notdef.
	return uint32(g.ID) != 0 && g.Bounds.Min.Y < g.Bounds.Max.Y
}

func requireMacGlyph(t *testing.T, glyphs []text.Glyph, face font.Typeface, value string) text.GlyphID {
	t.Helper()
	if len(glyphs) != 1 || !macGlyphHasInk(glyphs[0]) {
		t.Fatalf("expected one visible glyph for %q in %s, got %v", value, face, glyphs)
	}
	return glyphs[0].ID
}
