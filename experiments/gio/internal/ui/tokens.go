package ui

import (
	"time"

	"gioui.org/font"
	"gioui.org/unit"
)

// Spacing follows a 4dp rhythm, matching the Tailwind gaps used by Wails.
// Prefer these over literals so related controls stay aligned.
const (
	Space2  unit.Dp = 2
	Space4  unit.Dp = 4
	Space6  unit.Dp = 6
	Space8  unit.Dp = 8
	Space12 unit.Dp = 12
	Space16 unit.Dp = 16
	Space20 unit.Dp = 20
	Space24 unit.Dp = 24
	Space32 unit.Dp = 32
)

// Corner radii mirror --radius-* in frontend/src/index.css. Controls use
// RadiusLG, cards RadiusXL, floating layers RadiusXL and chips the full pill.
const (
	RadiusSM  unit.Dp = 6
	RadiusMD  unit.Dp = 8
	RadiusLG  unit.Dp = 10
	RadiusXL  unit.Dp = 12
	Radius2XL unit.Dp = 16
	// RadiusFull is larger than any control height so a rounded rectangle
	// degenerates to a pill.
	RadiusFull unit.Dp = 999
)

// Control heights. Every single-line control picks one of these so a button,
// a field and a select sit on the same baseline when placed in one row.
const (
	ControlCompact unit.Dp = 28 // segmented options, dense tables
	ControlSmall   unit.Dp = 32
	ControlDefault unit.Dp = 36
	ControlLarge   unit.Dp = 40
)

// Interaction durations. Wails transitions are 150ms for color and 200ms for
// layers that move or fade in.
const (
	DurationFast  = 100 * time.Millisecond
	DurationBase  = 150 * time.Millisecond
	DurationLayer = 200 * time.Millisecond
	TooltipDelay  = 500 * time.Millisecond
	TooltipFadeIn = 120 * time.Millisecond
)

// PressedScale is the scale of a pressed button; DisabledOpacity is the
// opacity of a disabled control (disabled:opacity-50 in Wails).
const (
	PressedScale    = 0.98
	DisabledOpacity = 0.5
)

// TextStyle is one step of the type scale.
type TextStyle struct {
	Size   unit.Sp
	Weight font.Weight
	// LineHeight is a multiple of Size.
	LineHeight float32
}

// The type scale follows Wails: sentence case, 14sp body, tabular figures in
// tables, and a larger serif only for page titles and hero figures.
var (
	TextCaption  = TextStyle{Size: 12, Weight: font.Normal, LineHeight: 1.4}
	TextCaption2 = TextStyle{Size: 12, Weight: font.Medium, LineHeight: 1.4}
	TextBody     = TextStyle{Size: 14, Weight: font.Normal, LineHeight: 1.45}
	TextLabel    = TextStyle{Size: 14, Weight: font.Medium, LineHeight: 1.4}
	TextHeading  = TextStyle{Size: 16, Weight: font.SemiBold, LineHeight: 1.3}
	TextTitle    = TextStyle{Size: 20, Weight: font.SemiBold, LineHeight: 1.25}
	TextPage     = TextStyle{Size: 24, Weight: font.Medium, LineHeight: 1.2}
	TextHero     = TextStyle{Size: 32, Weight: font.Medium, LineHeight: 1.1}
)

// ShadowLayer is one CSS box-shadow. Spread and blur follow CSS semantics, in
// dp, with the shadow offset downward by OffsetY.
type ShadowLayer struct {
	OffsetY, Blur, Spread unit.Dp
	Alpha                 float64
}

// Elevation names a step of the shadow scale. Structure comes from hairlines;
// shadows only separate floating layers and small hover lifts.
type Elevation uint8

const (
	ElevationNone Elevation = iota
	ElevationXS
	ElevationSM
	ElevationMD
	ElevationLG
)

// ShadowLayers returns the layers for an elevation. Dark appearances use heavier
// layers, as in the .dark overrides of --shadow-* in index.css.
func (p *Palette) ShadowLayers(level Elevation) []ShadowLayer {
	if p.Dark {
		return darkShadows[level]
	}
	return lightShadows[level]
}

var lightShadows = [...][]ShadowLayer{
	ElevationNone: nil,
	ElevationXS:   {{1, 1, 0, 0.04}},
	ElevationSM:   {{1, 2, 0, 0.05}},
	ElevationMD:   {{10, 24, -12, 0.18}, {2, 6, -3, 0.08}},
	ElevationLG:   {{24, 48, -18, 0.24}, {6, 14, -6, 0.10}},
}

var darkShadows = [...][]ShadowLayer{
	ElevationNone: nil,
	ElevationXS:   {{1, 2, 0, 0.24}},
	ElevationSM:   {{1, 3, 0, 0.32}, {1, 2, -1, 0.28}},
	ElevationMD:   {{8, 20, -6, 0.42}, {2, 6, -2, 0.32}},
	ElevationLG:   {{20, 40, -14, 0.52}, {4, 10, -4, 0.36}},
}
