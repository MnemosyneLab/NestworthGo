package ui

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/widget/material"
)

// The preview uses only fonts installed with the operating system. Each name
// is a Gio typeface list. Keep the primary Latin families first, then name
// the installed Han and emoji families before Gio's automatic script fallback.
// On macOS the automatic lookup can choose a missing-character box for Han
// text at medium and bold weights, and omit emoji even at regular weight.
//
// Plus Jakarta Sans, which Wails bundles, cannot be loaded here. Avenir Next
// is the closest macOS family with the same geometric proportions and tabular
// figures. Iowan Old Style is the first available serif in the Wails
// font-display stack. Both keep every digit the same width, so amounts line up
// without OpenType features. Georgia is excluded because its numerals are
// proportional and old-style. Where none of these exist, Gio uses its bundled
// default, which also has tabular figures.
const (
	SansTypeface  font.Typeface = "Avenir Next, Helvetica Neue, Hiragino Sans GB, Apple Color Emoji"
	SerifTypeface font.Typeface = "Iowan Old Style, Charter, Hiragino Sans GB, Apple Color Emoji"
)

// NewTheme returns a Material theme whose text and editors use SansTypeface.
// Every window creates its theme here so no text falls back to the toolkit
// default by accident.
func NewTheme() *material.Theme {
	theme := material.NewTheme()
	theme.Face = SansTypeface
	return theme
}

// Body is a wrapping paragraph in the given style.
func (v View) Body(style TextStyle, content string, fg color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return v.styled(gtx, style, content, fg, 0, text.Start)
	}
}

// Line is a single line in the given style that truncates instead of wrapping.
func (v View) Line(gtx layout.Context, style TextStyle, content string, fg color.NRGBA) layout.Dimensions {
	return v.styled(gtx, style, content, fg, 1, text.Start)
}

// LineEnd is Line aligned to the trailing edge, for amounts in a column.
func (v View) LineEnd(gtx layout.Context, style TextStyle, content string, fg color.NRGBA) layout.Dimensions {
	return v.styled(gtx, style, content, fg, 1, text.End)
}

func (v View) styled(gtx layout.Context, style TextStyle, content string, fg color.NRGBA, maxLines int, alignment text.Alignment) layout.Dimensions {
	label := material.Label(v.Theme, style.Size, content)
	label.Color = fg
	label.Font.Weight = faceWeight(style.Weight, content)
	label.LineHeightScale = style.LineHeight
	label.MaxLines = maxLines
	label.Alignment = alignment
	if style.Serif {
		label.Font.Typeface = SerifTypeface
	}
	return label.Layout(gtx)
}

// faceWeight maps a semibold request to bold when the text has non-Latin
// characters. The system face macOS picks for semibold Han text reports a
// line box with a very deep descent, which lifts the glyphs well above the
// center of every control they sit in. The bold face has regular metrics.
func faceWeight(weight font.Weight, content string) font.Weight {
	if weight != font.SemiBold {
		return weight
	}
	for _, r := range content {
		if r >= 0x2e80 {
			return font.Bold
		}
	}
	return weight
}
