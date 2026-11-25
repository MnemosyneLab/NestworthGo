package ui

import (
	"image/color"
	"math"
)

// OKLCH returns the sRGB color for a CSS oklch(L C h) value, with lightness as
// a fraction (0.45 for 45%) and hue in degrees. Out-of-gamut colors are
// clipped per channel, as browsers do for the Wails tokens in
// frontend/src/index.css. Keeping the source notation lets a token be compared
// with its Wails counterpart by eye.
func OKLCH(lightness, chroma, hue float64) color.NRGBA {
	radians := hue * math.Pi / 180
	a, b := chroma*math.Cos(radians), chroma*math.Sin(radians)
	l := cube(lightness + 0.3963377774*a + 0.2158037573*b)
	m := cube(lightness - 0.1055613458*a - 0.0638541728*b)
	s := cube(lightness - 0.0894841775*a - 1.2914855480*b)
	return color.NRGBA{
		R: encodeSRGB(4.0767416621*l - 3.3077115913*m + 0.2309699292*s),
		G: encodeSRGB(-1.2684380046*l + 2.6097574011*m - 0.3413193965*s),
		B: encodeSRGB(-0.0041960863*l - 0.7034186147*m + 1.7076147010*s),
		A: 0xff,
	}
}

func cube(v float64) float64 { return v * v * v }

func encodeSRGB(linear float64) uint8 {
	linear = math.Max(0, math.Min(1, linear))
	if linear <= 0.0031308 {
		return uint8(math.Round(12.92 * linear * 255))
	}
	return uint8(math.Round((1.055*math.Pow(linear, 1/2.4) - 0.055) * 255))
}

func decodeSRGB(value uint8) float64 {
	v := float64(value) / 255
	if v <= 0.04045 {
		return v / 12.92
	}
	return math.Pow((v+0.055)/1.055, 2.4)
}

// Mix blends over onto base with the given fraction of over (0 to 1). It
// behaves like CSS color-mix in sRGB for opaque colors and keeps base opaque.
// Wails builds its tinted surfaces this way, for example bg-primary/10.
func Mix(base, over color.NRGBA, fraction float64) color.NRGBA {
	fraction = math.Max(0, math.Min(1, fraction))
	blend := func(b, o uint8) uint8 {
		return uint8(math.Round(float64(b)*(1-fraction) + float64(o)*fraction))
	}
	return color.NRGBA{R: blend(base.R, over.R), G: blend(base.G, over.G), B: blend(base.B, over.B), A: base.A}
}

// WithAlpha returns c with its alpha channel set to fraction of full opacity.
func WithAlpha(c color.NRGBA, fraction float64) color.NRGBA {
	c.A = uint8(math.Round(math.Max(0, math.Min(1, fraction)) * 255))
	return c
}

// Luminance is the WCAG relative luminance of an opaque color.
func Luminance(c color.NRGBA) float64 {
	return 0.2126*decodeSRGB(c.R) + 0.7152*decodeSRGB(c.G) + 0.0722*decodeSRGB(c.B)
}

// Contrast is the WCAG contrast ratio between two opaque colors, from 1 to 21.
func Contrast(a, b color.NRGBA) float64 {
	la, lb := Luminance(a), Luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}
