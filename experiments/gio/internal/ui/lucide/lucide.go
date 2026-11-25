// Package lucide draws the Lucide icon set, which the Wails frontend also uses.
// Gio has no SVG renderer, so tools/lucidegen flattens each icon into numeric
// path tables and an Icon strokes them with round caps and joins, as Lucide
// specifies.
package lucide

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// Path command codes in the generated tables. Each command is followed by its
// coordinates: a move or line takes one point, a cubic three, a close none.
const (
	cmdMove = iota
	cmdLine
	cmdCubic
	cmdClose
)

// gridSize is Lucide's 24-unit design grid and strokeUnits its default stroke.
const (
	gridSize    = 24
	strokeUnits = 2
)

// Icon is one Lucide glyph. It satisfies the Gio UI glyph interface.
type Icon struct {
	paths [][]float32
}

// Layout draws the icon in the square given by gtx.Constraints.Min and strokes
// it in ink. The stroke scales with the icon, as the SVG viewBox does.
func (i *Icon) Layout(gtx layout.Context, ink color.NRGBA) layout.Dimensions {
	side := min(gtx.Constraints.Min.X, gtx.Constraints.Min.Y)
	if side == 0 {
		side = min(gtx.Constraints.Max.X, gtx.Constraints.Max.Y)
	}
	size := image.Pt(side, side)
	scale := float32(side) / gridSize

	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	var path clip.Path
	path.Begin(gtx.Ops)
	for _, data := range i.paths {
		trace(&path, data, scale)
	}
	stroke := clip.Stroke{Path: path.End(), Width: strokeUnits * scale}.Op().Push(gtx.Ops)
	paint.ColorOp{Color: ink}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	stroke.Pop()
	return layout.Dimensions{Size: size}
}

func trace(path *clip.Path, data []float32, scale float32) {
	point := func(i int) f32.Point { return f32.Pt(data[i]*scale, data[i+1]*scale) }
	for i := 0; i < len(data); {
		switch int(data[i]) {
		case cmdMove:
			path.MoveTo(point(i + 1))
			i += 3
		case cmdLine:
			path.LineTo(point(i + 1))
			i += 3
		case cmdCubic:
			path.CubeTo(point(i+1), point(i+3), point(i+5))
			i += 7
		default:
			path.Close()
			i++
		}
	}
}
