package charts

import (
	"image"
	"image/color"
	"math"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

// Slice is one composition entry.
type Slice struct {
	ID    ID
	Label string
	// Value sizes the arc. Only a finite, strictly positive value is drawn. A
	// negative value is never mirrored into a positive arc, and a zero or
	// missing value has no arc; the legend still lists the entry with its text.
	Value Value
	// Text is the exact amount and Share the exact percentage, as the caller
	// formats them. The arc angle is computed from Value alone, so rounding in
	// Share cannot change the drawing and the drawing cannot change Share.
	Text, Share string
	Detail      []string
	// Color overrides the theme category color for this entry.
	Color color.NRGBA
}

// DonutSpec describes a composition ring with a legend. Slices appear in the
// order given. The chart does not sort them or merge small ones into another
// category, and it does not decide which side of the balance sheet a value
// belongs to; the owner passes the magnitudes it wants drawn.
type DonutSpec struct {
	Label, Summary string
	Slices         []Slice
	// CenterValue and CenterCaption are the caller's text for the ring hole,
	// for example the total and its caption.
	CenterValue, CenterCaption string
	// NotDrawn is the note on legend entries that have no arc, such as "Not
	// shown in the ring".
	NotDrawn string
	// ZeroTotal replaces the caption when no entry can be drawn.
	ZeroTotal string
	Selected  Selection
	Messages  Messages
}

// Rows returns the text form of every slice in order, with the share appended
// to the details so a data table shows both.
func (s DonutSpec) Rows() []TextRow {
	rows := make([]TextRow, 0, len(s.Slices))
	for _, sl := range s.Slices {
		value, missing := valueText(sl.Value, sl.Text, s.Messages)
		detail := append([]string(nil), sl.Detail...)
		if sl.Share != "" {
			detail = append([]string{sl.Share}, detail...)
		}
		rows = append(rows, TextRow{
			ID: sl.ID, Label: sl.Label, Value: value, Detail: detail, Missing: missing,
			Selected: s.Selected == Selection{Item: sl.ID},
		})
	}
	return rows
}

// Validate lists empty and repeated slice IDs.
func (s DonutSpec) Validate() []Problem {
	var check idCheck
	for _, sl := range s.Slices {
		check.add("", sl.ID)
	}
	return check.problems
}

// donutGeometry are the pixel inputs of a plan.
type donutGeometry struct {
	width int
	// stacked puts the legend below the ring; otherwise it sits beside it.
	stacked  bool
	diameter int
	pullout  int
	// holeRatio is the inner radius as a fraction of the outer radius.
	holeRatio float64
	gap       int
	// legendRows are the measured legend row heights.
	legendRows []int
}

type donutSlicePlan struct {
	sel   Selection
	span  arcSpan
	drawn bool
	// row is the legend row.
	row image.Rectangle
}

type donutPlan struct {
	center       f32.Point
	outer, inner float64
	pullout      float64
	slices       []donutSlicePlan
	order        []Selection
	// total is the sum of the drawn values; zero when nothing can be drawn.
	total  float64
	legend image.Rectangle
	height int
}

func (s DonutSpec) plan(g donutGeometry) donutPlan {
	values := make([]Value, len(s.Slices))
	for i, sl := range s.Slices {
		values[i] = sl.Value
	}
	spans, total := ringShares(values)
	p := donutPlan{total: total, pullout: float64(g.pullout)}
	radius := float64(g.diameter) / 2
	p.outer = radius - p.pullout
	p.inner = p.outer * g.holeRatio
	p.center = f32.Pt(float32(radius), float32(radius))
	legendHeight := 0
	for _, h := range g.legendRows {
		legendHeight += h
	}
	if g.stacked {
		p.legend = image.Rect(0, g.diameter+g.gap, g.width, g.diameter+g.gap+legendHeight)
		p.center.X = float32(g.width) / 2
		p.height = p.legend.Max.Y
	} else {
		x := g.diameter + g.gap*2
		p.legend = image.Rect(x, 0, max(x, g.width), legendHeight)
		p.height = max(g.diameter, legendHeight)
	}
	y := p.legend.Min.Y
	for i, sl := range s.Slices {
		row := image.Rect(p.legend.Min.X, y, p.legend.Max.X, y+g.legendRows[i])
		y += g.legendRows[i]
		sp := donutSlicePlan{sel: Selection{Item: sl.ID}, span: spans[i], drawn: spans[i].sweep() > 0, row: row}
		p.slices = append(p.slices, sp)
		p.order = append(p.order, sp.sel)
	}
	return p
}

// onRing reports whether a point lies on the ring band.
func (p donutPlan) onRing(at image.Point) bool {
	dist := math.Hypot(float64(at.X)-float64(p.center.X), float64(at.Y)-float64(p.center.Y))
	return dist >= p.inner && dist <= p.outer+p.pullout
}

// pick finds the legend row or ring sector under a point.
func (p donutPlan) pick(at image.Point) (Selection, bool) {
	for _, s := range p.slices {
		if at.In(s.row) {
			return s.sel, true
		}
	}
	pt := f32.Pt(float32(at.X), float32(at.Y))
	dist := math.Hypot(float64(pt.X-p.center.X), float64(pt.Y-p.center.Y))
	if dist < p.inner || dist > p.outer+p.pullout {
		return Selection{}, false
	}
	angle := ringAngle(p.center, pt)
	for _, s := range p.slices {
		if s.drawn && angle >= s.span.start && angle < s.span.end {
			return s.sel, true
		}
	}
	return Selection{}, false
}

func (p donutPlan) sliceFor(sel Selection) (donutSlicePlan, int, bool) {
	for i, s := range p.slices {
		if s.sel == sel {
			return s, i, true
		}
	}
	return donutSlicePlan{}, 0, false
}

// sector is the outline of an annular sector, offset along its bisector by
// shift pixels.
func (p donutPlan) sector(span arcSpan, shift float64) []f32.Point {
	center := p.center
	if shift != 0 {
		mid := (span.start + span.end) / 2
		center = ringPoint(center, shift, mid)
	}
	outer := arcPolyline(center, p.outer, span.start, span.end)
	inner := arcPolyline(center, p.inner, span.end, span.start)
	return append(outer, inner...)
}

// DonutChart is the widget state of one donut chart.
type DonutChart struct {
	interaction
}

const (
	donutSideBySide unit.Dp = 560
	donutDiameter   unit.Dp = 240
	donutHole               = 0.64
)

// Layout draws the chart at the width of gtx.Constraints.Max and returns the
// selection requests made this frame. Below donutSideBySide the legend moves
// under the ring and every entry keeps its full width.
func (c *DonutChart) Layout(gtx layout.Context, view ui.View, spec DonutSpec) (layout.Dimensions, []Event) {
	width := gtx.Constraints.Max.X
	if len(spec.Slices) == 0 {
		return drawNotice(gtx, view, image.Pt(width, gtx.Dp(160)), spec.Messages.Empty), nil
	}
	pad := gtx.Dp(ringPad)
	inner := max(1, width-2*pad)
	colors := view.Colors
	stacked := !ui.WidthAtLeast(gtx, inner, donutSideBySide)
	gap := gtx.Dp(ui.Space12)
	diameter := min(gtx.Dp(donutDiameter), inner)
	if !stacked {
		diameter = min(diameter, inner*2/5)
	}
	g := donutGeometry{width: inner, stacked: stacked, diameter: diameter, pullout: gtx.Dp(ui.Space6), holeRatio: donutHole, gap: gap}
	legendWidth := inner
	if !stacked {
		legendWidth = inner - diameter - 2*gap
	}
	rowInset := gtx.Dp(ui.Space8)
	swatch := gtx.Dp(ui.Space12)
	columns := make([]donutRow, len(spec.Slices))
	rightW := 0
	for i, sl := range spec.Slices {
		columns[i].amount, _ = valueText(sl.Value, sl.Text, spec.Messages)
		rightW = max(rightW,
			textBlock{style: ui.TextLabel, content: columns[i].amount, maxWidth: legendWidth, maxLines: 1}.measure(gtx, view).X,
			textBlock{style: ui.TextCaption, content: sl.Share, maxWidth: legendWidth, maxLines: 1}.measure(gtx, view).X)
	}
	rightW = min(rightW, legendWidth*45/100)
	labelW := max(1, legendWidth-2*rowInset-swatch-gtx.Dp(ui.Space8)-rightW-gap)
	for i, sl := range spec.Slices {
		note := ""
		if !(sl.Value.Valid() && sl.Value.Number > 0) {
			note = spec.NotDrawn
		}
		columns[i].note = note
		labelH := textBlock{style: ui.TextBody, content: sl.Label, maxWidth: labelW}.measure(gtx, view).Y
		noteH := textBlock{style: ui.TextCaption, content: note, maxWidth: labelW}.measure(gtx, view).Y
		amountH := textBlock{style: ui.TextLabel, content: columns[i].amount, maxWidth: max(1, rightW)}.measure(gtx, view).Y +
			textBlock{style: ui.TextCaption, content: sl.Share, maxWidth: max(1, rightW)}.measure(gtx, view).Y
		g.legendRows = append(g.legendRows, max(labelH+noteH, amountH)+2*gtx.Dp(ui.Space8))
	}
	plan := spec.plan(g)

	items := frameItems{order: plan.order, selected: spec.Selected, pick: plan.pick}
	events := c.update(gtx, c, items)
	active := c.active(gtx, c, plan.order)

	dims := accessibility.List(gtx, c, spec.Label, func(gtx layout.Context) layout.Dimensions {
		defer op.Offset(image.Pt(pad, pad)).Push(gtx.Ops).Pop()
		hovering := !c.hover.IsZero()
		c.drawRing(gtx, view, spec, plan, active)
		c.drawCenter(gtx, view, spec, plan)
		c.drawLegend(gtx, view, spec, plan, columns, active, rowInset, swatch, labelW, rightW)
		y := plan.height + gtx.Dp(ui.Space8)
		rgtx := gtx
		rgtx.Constraints.Max.X = inner
		shown := active
		if shown.IsZero() {
			shown = spec.Selected
		}
		offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		read := layoutReadout(rgtx, view, spec.Messages, spec.Summary, spec.readout(shown))
		offset.Pop()
		y += read.Size.Y

		size := image.Pt(width, y+2*pad)
		func() {
			defer op.Offset(image.Pt(-pad, -pad)).Push(gtx.Ops).Pop()
			register(gtx, c, size, hovering)
			if gtx.Focused(c) {
				colors.FocusRing(gtx, image.Rectangle{Min: image.Pt(pad, pad), Max: size.Sub(image.Pt(pad, pad))}, ui.RadiusMD)
			}
		}()
		return layout.Dimensions{Size: size}
	})
	return dims, events
}

type donutRow struct{ amount, note string }

func (s DonutSpec) readout(sel Selection) *readout {
	if sel.IsZero() {
		return nil
	}
	for _, sl := range s.Slices {
		if sl.ID != sel.Item {
			continue
		}
		value, missing := valueText(sl.Value, sl.Text, s.Messages)
		if sl.Share != "" {
			value += " · " + sl.Share
		}
		detail := sl.Detail
		if !(sl.Value.Valid() && sl.Value.Number > 0) && s.NotDrawn != "" {
			detail = append(append([]string(nil), detail...), s.NotDrawn)
		}
		return &readout{label: sl.Label, value: value, detail: detail, selected: sel == s.Selected, missing: missing}
	}
	return nil
}

func (s DonutSpec) sliceInk(colors *ui.Palette, index int) color.NRGBA {
	if ink := s.Slices[index].Color; ink != (color.NRGBA{}) {
		return ink
	}
	return colors.Category[index%len(colors.Category)]
}

func (c *DonutChart) drawRing(gtx layout.Context, view ui.View, spec DonutSpec, plan donutPlan, active Selection) {
	colors := view.Colors
	if plan.total <= 0 {
		// Nothing can be drawn: show the empty track so the chart keeps its shape.
		track := plan.sector(arcSpan{start: 0, end: 2 * math.Pi}, 0)
		fillPolygon(gtx, track, colors.Tint(colors.Ink, 0.06))
		strokePolyline(gtx, append(track, track[0]), 1, colors.Divider)
	}
	gapWidth := float32(gtx.Dp(2))
	for i, sp := range plan.slices {
		if !sp.drawn {
			continue
		}
		shift := 0.0
		if sp.sel == spec.Selected {
			shift = plan.pullout
		}
		outline := plan.sector(sp.span, shift)
		fillPolygon(gtx, outline, spec.sliceInk(colors, i))
		if sp.span.sweep() < 2*math.Pi-1e-6 {
			strokePolyline(gtx, append(outline, outline[0]), gapWidth, colors.Surface)
		}
		if sp.sel == active || sp.sel == spec.Selected {
			// Selected and active sectors carry an ink outline.
			width, ink := float32(gtx.Dp(2)), colors.Ink
			if sp.sel != spec.Selected {
				ink = ui.WithAlpha(colors.Ink, 0.55)
			}
			strokePolyline(gtx, append(outline, outline[0]), width, ink)
		}
	}
	// The tooltip explains a sector the pointer is over or the keyboard cursor
	// is on. A hovered legend row already explains itself.
	showTip := (!c.hover.IsZero() && plan.onRing(c.at)) || (c.hover.IsZero() && gtx.Focused(c))
	if sp, _, ok := plan.sliceFor(active); ok && sp.drawn && showTip {
		if item := spec.readout(active); item != nil {
			mid := (sp.span.start + sp.span.end) / 2
			at := ringPoint(plan.center, plan.outer, mid)
			bounds := image.Rectangle{Max: image.Pt(int(plan.center.X*2), int(plan.center.Y*2))}
			drawTooltip(gtx, view, bounds, image.Pt(int(at.X), int(at.Y)), *item)
		}
	}
}

// drawCenter writes the caller's total in the hole. The value falls back to a
// smaller style and then wraps, so a long amount never leaves the hole.
func (c *DonutChart) drawCenter(gtx layout.Context, view ui.View, spec DonutSpec, plan donutPlan) {
	if spec.CenterValue == "" && spec.CenterCaption == "" {
		return
	}
	colors := view.Colors
	avail := max(1, int(plan.inner*2*0.8))
	value := textBlock{style: ui.TextMoneyMD, content: spec.CenterValue, ink: colors.Ink, maxWidth: avail, maxLines: 1, align: text.Middle}
	if value.natural(gtx, view) > avail {
		value.style = ui.TextLabel
	}
	if value.natural(gtx, view) > avail {
		// Wrap rather than truncate: the amount is the point of the hole.
		value.maxLines = 0
	}
	caption := spec.CenterCaption
	if plan.total <= 0 && spec.ZeroTotal != "" {
		caption = spec.ZeroTotal
	}
	capBlock := textBlock{style: ui.TextCaption, content: caption, ink: colors.SecondaryInk, maxWidth: avail, align: text.Middle}
	vSize, cSize := value.measure(gtx, view), capBlock.measure(gtx, view)
	total := vSize.Y + cSize.Y
	top := int(plan.center.Y) - total/2
	left := int(plan.center.X) - avail/2
	value.draw(gtx, view, image.Pt(left, top))
	capBlock.draw(gtx, view, image.Pt(left, top+vSize.Y))
}

func (c *DonutChart) drawLegend(gtx layout.Context, view ui.View, spec DonutSpec, plan donutPlan, columns []donutRow, active Selection, inset, swatch, labelW, rightW int) {
	colors := view.Colors
	for i, sp := range plan.slices {
		sl := spec.Slices[i]
		selected := sp.sel == spec.Selected
		switch {
		case selected:
			ui.FillRoundRect(gtx, sp.row, ui.RadiusSM, colors.Tint(colors.Accent, 0.10))
			ui.FillRoundRect(gtx, image.Rect(sp.row.Min.X, sp.row.Min.Y+gtx.Dp(4), sp.row.Min.X+gtx.Dp(3), sp.row.Max.Y-gtx.Dp(4)), ui.RadiusFull, colors.Accent)
		case sp.sel == active:
			ui.FillRoundRect(gtx, sp.row, ui.RadiusSM, colors.HoverSurface)
		}
		if sp.sel == active && gtx.Focused(c) {
			ui.StrokeRoundRect(gtx, sp.row, ui.RadiusSM, 2, ui.WithAlpha(colors.Accent, 0.8))
		}
		pad := gtx.Dp(ui.Space8)
		y := sp.row.Min.Y + pad
		swatchBox := image.Rect(sp.row.Min.X+inset, y+gtx.Dp(3), sp.row.Min.X+inset+swatch, y+gtx.Dp(3)+swatch)
		ink := spec.sliceInk(colors, i)
		if sp.drawn {
			ui.FillRoundRect(gtx, swatchBox, ui.RadiusSM/2, ink)
		} else {
			// A hollow swatch marks an entry with no arc.
			ui.StrokeRoundRect(gtx, swatchBox, ui.RadiusSM/2, 1, colors.SecondaryInk)
		}
		x := swatchBox.Max.X + gtx.Dp(ui.Space8)
		size := textBlock{style: ui.TextBody, content: sl.Label, ink: colors.Ink, maxWidth: labelW}.draw(gtx, view, image.Pt(x, y))
		textBlock{style: ui.TextCaption, content: columns[i].note, ink: colors.SecondaryInk, maxWidth: labelW}.draw(gtx, view, image.Pt(x, y+size.Y))
		rx := sp.row.Max.X - inset - rightW
		amountInk := colors.Ink
		if !sp.drawn && !sl.Value.Valid() {
			amountInk = colors.SecondaryInk
		}
		amount := textBlock{style: ui.TextLabel, content: columns[i].amount, ink: amountInk, maxWidth: max(1, rightW), align: text.End}.draw(gtx, view, image.Pt(rx, y))
		textBlock{style: ui.TextCaption, content: sl.Share, ink: colors.SecondaryInk, maxWidth: max(1, rightW), align: text.End}.draw(gtx, view, image.Pt(rx, y+amount.Y))
		if item := spec.readout(sp.sel); item != nil {
			semanticItem(gtx, sp.row, itemLabel(*item, spec.Messages))
		}
	}
}
