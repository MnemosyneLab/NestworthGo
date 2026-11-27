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
	// for example the total and its caption. A Compact ring leaves the hole
	// empty.
	CenterValue, CenterCaption string
	// CenterFit lists the totals that should use the same fitted size in
	// equally sized companion charts.
	CenterFit []string
	// NotDrawn is the note on legend entries that have no arc, such as "Not
	// shown in the ring".
	NotDrawn string
	// ZeroTotal replaces the caption when no entry can be drawn.
	ZeroTotal string
	Selected  Selection
	Messages  Messages
	// Compact fits the ring and a one-line legend into a card of about 360dp,
	// as the Wails overview does: each entry shows its share as a pill beside
	// the amount instead of under it.
	Compact bool
	// StackLegend gives the chart its own row and scales the ring to the available width.
	StackLegend bool
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
	return p.grownSector(span, shift, 0)
}

// grownSector is sector with the outer radius extended by grow pixels, the
// emphasis of an active slice.
func (p donutPlan) grownSector(span arcSpan, shift, grow float64) []f32.Point {
	center := p.center
	if shift != 0 {
		mid := (span.start + span.end) / 2
		center = ringPoint(center, shift, mid)
	}
	outer := arcPolyline(center, p.outer+grow, span.start, span.end)
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

	// A compact donut sits beside its legend from this width.
	donutCompactSideBySide unit.Dp = 330
	donutCompactDiameter   unit.Dp = 124
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
	sideBySide, diameterDp, gapDp := donutSideBySide, donutDiameter, ui.Space12
	if spec.Compact {
		sideBySide, diameterDp, gapDp = donutCompactSideBySide, donutCompactDiameter, ui.Space8
	}
	stacked := spec.StackLegend || !ui.WidthAtLeast(gtx, inner, sideBySide)
	gap := gtx.Dp(gapDp)
	diameter := min(gtx.Dp(diameterDp), inner)
	if spec.StackLegend {
		diameter = min(inner, max(gtx.Dp(200), min(gtx.Dp(300), inner*3/5)))
	} else if !stacked {
		diameter = min(diameter, inner*2/5)
	} else if spec.Compact {
		diameter = min(gtx.Dp(donutDiameter*2/3), inner)
	}
	g := donutGeometry{width: inner, stacked: stacked, diameter: diameter, pullout: gtx.Dp(ui.Space6), holeRatio: donutHole, gap: gap}
	legendWidth := inner
	if !stacked {
		legendWidth = inner - diameter - 2*gap
	}
	rowInset := gtx.Dp(ui.Space8)
	swatch := gtx.Dp(ui.Space12)
	rowPad := gtx.Dp(ui.Space8)
	if spec.Compact {
		rowInset, swatch, rowPad = gtx.Dp(ui.Space4), gtx.Dp(10), gtx.Dp(ui.Space6)
	}
	columns := make([]donutRow, len(spec.Slices))
	rightW := 0
	for i, sl := range spec.Slices {
		columns[i].amount, _ = valueText(sl.Value, sl.Text, spec.Messages)
		if !(sl.Value.Valid() && sl.Value.Number > 0) {
			columns[i].note = spec.NotDrawn
		}
		amount := textBlock{style: ui.TextLabel, content: columns[i].amount, maxWidth: legendWidth, maxLines: 1}.measure(gtx, view).X
		share := textBlock{style: ui.TextCaption, content: sl.Share, maxWidth: legendWidth, maxLines: 1}.measure(gtx, view).X
		rightW = max(rightW, amount, share)
	}
	rightW = min(rightW, legendWidth*45/100)
	labelW := max(1, legendWidth-2*rowInset-swatch-gtx.Dp(ui.Space8)-rightW-gap)
	var compact compactLegend
	if spec.Compact {
		compact = newCompactLegend(gtx, view, spec, columns, legendWidth, rowInset, swatch)
	}
	for i, sl := range spec.Slices {
		if spec.Compact {
			g.legendRows = append(g.legendRows, compact.rowHeight(gtx, view, sl, columns[i])+2*rowPad)
			continue
		}
		labelH := textBlock{style: ui.TextBody, content: sl.Label, maxWidth: labelW}.measure(gtx, view).Y
		noteH := textBlock{style: ui.TextCaption, content: columns[i].note, maxWidth: labelW}.measure(gtx, view).Y
		amountH := textBlock{style: ui.TextLabel, content: columns[i].amount, maxWidth: max(1, rightW)}.measure(gtx, view).Y
		amountH += textBlock{style: ui.TextCaption, content: sl.Share, maxWidth: max(1, rightW)}.measure(gtx, view).Y
		g.legendRows = append(g.legendRows, max(labelH+noteH, amountH)+2*rowPad)
	}
	plan := spec.plan(g)

	items := frameItems{order: plan.order, selected: spec.Selected, pick: plan.pick}
	events := c.update(gtx, c, items)
	active := c.active(gtx, c, plan.order)

	dims := accessibility.List(gtx, c, spec.Label, func(gtx layout.Context) layout.Dimensions {
		defer op.Offset(image.Pt(pad, pad)).Push(gtx.Ops).Pop()
		hovering := !c.hover.IsZero()
		c.drawRing(gtx, view, spec, plan, active, image.Rect(0, 0, inner, plan.height))
		c.drawCenter(gtx, view, spec, plan)
		c.drawLegend(gtx, view, spec, plan, columns, active, legendMetrics{inset: rowInset, swatch: swatch, pad: rowPad, labelW: labelW, rightW: rightW, compact: compact})
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
	return colors.ChartInk(index)
}

func (c *DonutChart) drawRing(gtx layout.Context, view ui.View, spec DonutSpec, plan donutPlan, active Selection, bounds image.Rectangle) {
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
		shift, grow := 0.0, 0.0
		if sp.sel == spec.Selected {
			shift = plan.pullout
		}
		if sp.sel == active {
			// The active slice grows outward; the others stay put, so the
			// ring keeps its shape.
			grow = float64(gtx.Dp(ui.Space4))
		}
		outline := plan.grownSector(sp.span, shift, grow)
		ink := spec.sliceInk(colors, i)
		if !active.IsZero() && sp.sel != active && sp.sel != spec.Selected {
			ink = ui.Mix(colors.Surface, ink, 0.7)
		}
		fillPolygon(gtx, outline, ink)
		if sp.span.sweep() < 2*math.Pi-1e-6 {
			strokePolyline(gtx, append(outline, outline[0]), gapWidth, colors.Surface)
		}
		if sp.sel == spec.Selected {
			// The selected sector carries an ink outline, so selection does
			// not rely on color or position alone.
			strokePolyline(gtx, append(outline, outline[0]), float32(gtx.Dp(2)), colors.Ink)
		}
	}
	// The tooltip explains a sector the pointer is over or the keyboard cursor
	// is on. A hovered legend row already explains itself.
	showTip := (!c.hover.IsZero() && plan.onRing(c.at)) || (c.hover.IsZero() && gtx.Focused(c))
	if sp, index, ok := plan.sliceFor(active); ok && sp.drawn && showTip {
		if item := spec.readout(active); item != nil {
			item.mark = readoutMark{ink: spec.sliceInk(colors, index), shape: markerSquare}
			mid := (sp.span.start + sp.span.end) / 2
			at := ringPoint(plan.center, plan.outer+float64(gtx.Dp(ui.Space4)), mid)
			drawTooltip(gtx, view, bounds, image.Pt(int(at.X), int(at.Y)), *item)
		}
	}
}

// drawCenter writes the caller's total in the hole. The value falls back to a
// smaller style and then wraps, so a long amount never leaves the hole.
func (c *DonutChart) drawCenter(gtx layout.Context, view ui.View, spec DonutSpec, plan donutPlan) {
	if spec.CenterValue == "" && spec.CenterCaption == "" || spec.Compact {
		// A compact ring has no room for an amount; the owner shows it beside
		// the ring.
		return
	}
	colors := view.Colors
	avail := max(1, int(plan.inner*2*0.8))
	value := textBlock{style: ui.TextMoneyMD, content: spec.CenterValue, ink: colors.Ink, maxWidth: avail, maxLines: 1, align: text.Middle}
	if spec.CenterValue == spec.Messages.Missing {
		value.style = ui.TextMetricMD
	}
	value.style = donutCenterStyle(gtx, view, value, spec.CenterFit)
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

func donutCenterStyle(gtx layout.Context, view ui.View, value textBlock, totals []string) ui.TextStyle {
	width := value.natural(gtx, view)
	for _, total := range totals {
		if total != value.content {
			peer := value
			peer.content = total
			width = max(width, peer.natural(gtx, view))
		}
	}
	if width > value.maxWidth {
		// Keep the money face and weight when there is less room.
		fitted := math.Floor(float64(value.style.Size) * float64(value.maxWidth) / float64(width))
		value.style.Size = max(ui.TextLabel.Size, unit.Sp(fitted))
	}
	return value.style
}

// legendMetrics are the measured pixel sizes of the legend columns. A
// compact legend uses its own budget instead of labelW and rightW.
type legendMetrics struct {
	inset, swatch, pad, labelW, rightW int
	compact                            compactLegend
}

const (
	// compactFiguresShare is the most of a compact row's text width the
	// amount and share may take beside the category.
	compactFiguresShare = 65
	// compactLabelMin is the narrowest a category may become beside the
	// figures before the legend stacks them under it.
	compactLabelMin unit.Dp = 88
)

// compactLegend is the single width budget of a compact legend. Row heights,
// positions and drawing all read it, so what is measured is what is drawn.
// Every row uses the same arrangement, so the figures stay in one column.
type compactLegend struct {
	// stacked puts the amount and share on their own line under the
	// category, used when they do not fit beside it. chipBelow further moves
	// the share chip under the amount when the two do not share a line.
	stacked, chipBelow bool
	// labelX is the text start from the row's leading edge; labelW is the
	// category's wrap width.
	labelX, labelW int
	// amountW is the amount's width: its natural width beside the label,
	// or the line minus the share chip when stacked, where it wraps rather
	// than being cut. chipW is the share chip's width, zero without shares.
	amountW, chipW int
	// inset is the row's trailing inset, chipGap the space before the chip
	// and chipPad the chip's horizontal padding.
	inset, chipGap, chipPad int
}

func newCompactLegend(gtx layout.Context, view ui.View, spec DonutSpec, columns []donutRow, legendWidth, inset, swatch int) compactLegend {
	c := compactLegend{labelX: inset + swatch + gtx.Dp(ui.Space6), inset: inset, chipPad: gtx.Dp(ui.Space6)}
	textW := max(1, legendWidth-c.labelX-inset)
	for i, sl := range spec.Slices {
		c.amountW = max(c.amountW, textBlock{style: ui.TextLabel, content: columns[i].amount, maxWidth: 1 << 16, maxLines: 1}.natural(gtx, view))
		if sl.Share != "" {
			c.chipW = max(c.chipW, textBlock{style: ui.TextCaption, content: sl.Share, maxWidth: 1 << 16, maxLines: 1}.natural(gtx, view)+2*c.chipPad)
		}
	}
	if c.chipW > 0 {
		c.chipGap = gtx.Dp(ui.Space6)
	}
	figures := c.amountW + c.chipGap + c.chipW
	beside := textW - figures - gtx.Dp(ui.Space8)
	if figures <= textW*compactFiguresShare/100 && beside >= gtx.Dp(compactLabelMin) {
		c.labelW = beside
		return c
	}
	c.stacked, c.labelW = true, textW
	c.chipW = min(c.chipW, textW)
	if figures > textW {
		c.chipBelow, c.amountW = c.chipW > 0, textW
		return c
	}
	c.amountW = max(1, textW-c.chipGap-c.chipW)
	return c
}

func (c compactLegend) amountBlock(amount string, ink color.NRGBA) textBlock {
	block := textBlock{style: ui.TextLabel, content: amount, ink: ink, maxWidth: max(1, c.amountW), maxLines: 1, align: text.End}
	if c.stacked {
		block.maxLines, block.align = 0, text.Start
	}
	return block
}

func (c compactLegend) chipBlock(share string, ink color.NRGBA) textBlock {
	return textBlock{style: ui.TextCaption, content: share, ink: ink, maxWidth: max(1, c.chipW-2*c.chipPad), maxLines: 1, align: text.Middle}
}

func (c compactLegend) chipHeight(gtx layout.Context, view ui.View, share string) int {
	if c.chipW == 0 || share == "" {
		return 0
	}
	return c.chipBlock(share, color.NRGBA{}).measure(gtx, view).Y + 2*gtx.Dp(2)
}

func (c compactLegend) textHeight(gtx layout.Context, view ui.View, sl Slice, row donutRow) int {
	return textBlock{style: ui.TextBody, content: sl.Label, maxWidth: c.labelW}.measure(gtx, view).Y +
		textBlock{style: ui.TextCaption, content: row.note, maxWidth: c.labelW}.measure(gtx, view).Y
}

func (c compactLegend) figuresHeight(gtx layout.Context, view ui.View, sl Slice, row donutRow) int {
	amountH, chipH := c.amountBlock(row.amount, color.NRGBA{}).measure(gtx, view).Y, c.chipHeight(gtx, view, sl.Share)
	if c.chipBelow && chipH > 0 {
		return amountH + gtx.Dp(ui.Space2) + chipH
	}
	return max(amountH, chipH)
}

// rowHeight is the content height of one entry, without the row padding.
func (c compactLegend) rowHeight(gtx layout.Context, view ui.View, sl Slice, row donutRow) int {
	textH, figuresH := c.textHeight(gtx, view, sl, row), c.figuresHeight(gtx, view, sl, row)
	if c.stacked {
		return textH + gtx.Dp(ui.Space2) + figuresH
	}
	return max(textH, figuresH)
}

func (c *DonutChart) drawLegend(gtx layout.Context, view ui.View, spec DonutSpec, plan donutPlan, columns []donutRow, active Selection, m legendMetrics) {
	colors := view.Colors
	inset, swatch, rightW := m.inset, m.swatch, m.rightW
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
		offset := op.Offset(sp.row.Min).Push(gtx.Ops)
		row := image.Rectangle{Max: sp.row.Size()}
		legend := func(gtx layout.Context) layout.Dimensions {
			y := m.pad
			swatchBox := image.Rect(inset, y+gtx.Dp(3), inset+swatch, y+gtx.Dp(3)+swatch)
			ink := spec.sliceInk(colors, i)
			if sp.drawn {
				ui.FillRoundRect(gtx, swatchBox, ui.RadiusSM/2, ink)
			} else {
				// A hollow swatch marks an entry with no arc.
				ui.StrokeRoundRect(gtx, swatchBox, ui.RadiusSM/2, 1, colors.SecondaryInk)
			}
			x, labelW := swatchBox.Max.X+gtx.Dp(ui.Space8), m.labelW
			if spec.Compact {
				x, labelW = m.compact.labelX, m.compact.labelW
			}
			size := textBlock{style: ui.TextBody, content: sl.Label, ink: colors.Ink, maxWidth: labelW}.draw(gtx, view, image.Pt(x, y))
			textBlock{style: ui.TextCaption, content: columns[i].note, ink: colors.SecondaryInk, maxWidth: labelW}.draw(gtx, view, image.Pt(x, y+size.Y))
			amountInk := colors.Ink
			if !sp.drawn && !sl.Value.Valid() {
				amountInk = colors.SecondaryInk
			}
			if spec.Compact {
				m.compact.drawFigures(gtx, view, row, m.pad, sl, columns[i], amountInk)
			} else {
				rx := row.Max.X - inset - rightW
				amount := textBlock{style: ui.TextLabel, content: columns[i].amount, ink: amountInk, maxWidth: max(1, rightW), align: text.End}.draw(gtx, view, image.Pt(rx, y))
				textBlock{style: ui.TextCaption, content: sl.Share, ink: colors.SecondaryInk, maxWidth: max(1, rightW), align: text.End}.draw(gtx, view, image.Pt(rx, y+amount.Y))
			}
			return layout.Dimensions{Size: row.Size()}
		}
		if item := spec.readout(sp.sel); item != nil {
			accessibility.Summary(gtx, itemLabel(*item, spec.Messages), legend)
		} else {
			legend(gtx)
		}
		offset.Pop()
	}
}

// drawFigures puts the amount and the share chip on the row's centre line,
// flush with the trailing edge, or on the line under the category when the
// legend is stacked.
func (c compactLegend) drawFigures(gtx layout.Context, view ui.View, row image.Rectangle, pad int, sl Slice, entry donutRow, ink color.NRGBA) {
	colors := view.Colors
	figuresH := c.figuresHeight(gtx, view, sl, entry)
	top := row.Min.Y + (row.Dy()-figuresH)/2
	if c.stacked {
		top = row.Min.Y + pad + c.textHeight(gtx, view, sl, entry) + gtx.Dp(ui.Space2)
	}
	right := row.Max.X - c.inset
	block := c.amountBlock(entry.amount, ink)
	if chipH := c.chipHeight(gtx, view, sl.Share); chipH > 0 {
		chipTop := top + (figuresH-chipH)/2
		chip := image.Rect(right-c.chipW, chipTop, right, chipTop+chipH)
		switch {
		case c.chipBelow:
			chipTop = top + block.measure(gtx, view).Y + gtx.Dp(ui.Space2)
			left := row.Min.X + c.labelX
			chip = image.Rect(left, chipTop, left+c.chipW, chipTop+chipH)
		case c.stacked:
			chip = image.Rect(right-c.chipW, top, right, top+chipH)
		}
		ui.FillRoundRect(gtx, chip, ui.RadiusFull, colors.HoverSurface)
		c.chipBlock(sl.Share, colors.SecondaryInk).draw(gtx, view, image.Pt(chip.Min.X+c.chipPad, chip.Min.Y+gtx.Dp(2)))
	}
	if c.stacked {
		block.draw(gtx, view, image.Pt(row.Min.X+c.labelX, top))
		return
	}
	block.draw(gtx, view, image.Pt(right-c.chipW-c.chipGap-c.amountW, top+(figuresH-block.measure(gtx, view).Y)/2))
}
