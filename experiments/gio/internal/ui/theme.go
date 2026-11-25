package ui

import (
	"image/color"

	"gioui.org/widget"
)

// Palette holds the resolved colors for one color theme and appearance.
//
// The first group is the accent-specific surface vocabulary. Its names map to
// the Wails tokens in frontend/src/index.css:
//
//	PageBackground = background      Surface      = card
//	SidebarBackground = sidebar      HoverSurface = muted
//	Divider = border                 SecondaryInk = muted-foreground
//	Ink = foreground                 Accent       = primary
//
// The second group is identical for every accent so gain, loss and data
// provenance never depend on the chosen color theme. Call WithSemantics once
// per palette to fill it.
type Palette struct {
	PageBackground    color.NRGBA
	SidebarBackground color.NRGBA
	Surface           color.NRGBA
	HoverSurface      color.NRGBA
	Divider           color.NRGBA
	Ink               color.NRGBA
	SecondaryInk      color.NRGBA
	Accent            color.NRGBA
	OnAccent          color.NRGBA
	ErrorInk          color.NRGBA

	// Dark selects the dark elevation and tint rules.
	Dark bool

	Success, OnSuccess         color.NRGBA
	Warning, WarningInk        color.NRGBA
	Destructive, OnDestructive color.NRGBA
	// ShadowTint is the shadow color before layer alpha is applied.
	ShadowTint color.NRGBA
	Provenance Provenance
	// Category colors tint account types, chart series and icon tiles.
	Category [8]color.NRGBA
}

// Provenance colors mark where a value came from. Color is never the only
// signal; callers pair it with a label.
type Provenance struct {
	Current, Manual, Remote, Stale, Unavailable color.NRGBA
}

// WithSemantics returns p with the shared status, provenance, category and
// shadow tokens for the given appearance. The values are the OKLCH tokens in
// frontend/src/index.css.
func (p Palette) WithSemantics(dark bool) Palette {
	p.Dark = dark
	if !dark {
		p.Success, p.OnSuccess = OKLCH(0.58, 0.15, 155), OKLCH(0.99, 0.01, 155)
		p.Warning, p.WarningInk = OKLCH(0.80, 0.15, 80), OKLCH(0.38, 0.09, 65)
		p.Destructive, p.OnDestructive = OKLCH(0.60, 0.20, 22), OKLCH(0.99, 0.01, 22)
		p.ShadowTint = OKLCH(0.30, 0.03, 70)
		p.Provenance = Provenance{
			Current:     OKLCH(0.58, 0.15, 155),
			Manual:      OKLCH(0.60, 0.15, 295),
			Remote:      OKLCH(0.64, 0.12, 235),
			Stale:       OKLCH(0.78, 0.15, 80),
			Unavailable: OKLCH(0.64, 0.02, 200),
		}
		p.Category = [8]color.NRGBA{
			OKLCH(0.64, 0.15, 245), OKLCH(0.66, 0.14, 165), OKLCH(0.66, 0.18, 25), OKLCH(0.80, 0.15, 90),
			OKLCH(0.64, 0.16, 295), OKLCH(0.73, 0.15, 55), OKLCH(0.65, 0.19, 350), OKLCH(0.62, 0.11, 210),
		}
		return p
	}
	p.Success, p.OnSuccess = OKLCH(0.76, 0.15, 155), OKLCH(0.16, 0.03, 155)
	p.Warning, p.WarningInk = OKLCH(0.82, 0.14, 82), OKLCH(0.84, 0.12, 82)
	p.Destructive, p.OnDestructive = OKLCH(0.72, 0.18, 22), OKLCH(0.16, 0.03, 22)
	p.ShadowTint = color.NRGBA{A: 0xff}
	p.Provenance = Provenance{
		Current:     OKLCH(0.76, 0.15, 155),
		Manual:      OKLCH(0.75, 0.13, 295),
		Remote:      OKLCH(0.76, 0.11, 235),
		Stale:       OKLCH(0.82, 0.14, 82),
		Unavailable: OKLCH(0.68, 0.02, 200),
	}
	p.Category = [8]color.NRGBA{
		OKLCH(0.76, 0.13, 245), OKLCH(0.77, 0.13, 165), OKLCH(0.75, 0.15, 25), OKLCH(0.84, 0.14, 92),
		OKLCH(0.75, 0.14, 295), OKLCH(0.80, 0.13, 55), OKLCH(0.75, 0.16, 350), OKLCH(0.74, 0.10, 210),
	}
	return p
}

// Tint draws c over the card surface at the given strength. It is the Gio
// counterpart of a Tailwind utility such as bg-primary/10.
func (p *Palette) Tint(c color.NRGBA, strength float64) color.NRGBA {
	return Mix(p.Surface, c, strength)
}

// CategorySoft is the quiet background that pairs with a Category color.
func (p *Palette) CategorySoft(index int) color.NRGBA {
	strength := 0.16
	if index == 3 {
		strength = 0.20
	}
	return Mix(p.Surface, p.Category[index%len(p.Category)], strength)
}

func (p Palette) EditorSelection() color.NRGBA {
	selection := p.Accent
	// Keep the editor's text readable on the bright accents in dark mode.
	selection.A = 0x48
	return selection
}

func MustIcon(iconData []byte) *widget.Icon {
	icon, err := widget.NewIcon(iconData)
	if err != nil {
		panic(err)
	}
	return icon
}
