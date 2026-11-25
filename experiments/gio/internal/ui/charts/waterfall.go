package charts

import (
	"image"
	"math"
	"strconv"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

// StepKind says how a waterfall step is anchored.
type StepKind uint8

const (
	// StepDelta floats between From and To, both supplied by the caller.
	StepDelta StepKind = iota
	// StepTotal stands on the zero baseline and rises or falls to To, as a
	// beginning or ending value does. From is ignored.
	StepTotal
)

// WaterfallStep is one column of a waterfall. The caller supplies the levels at
// both ends of every step. The chart never accumulates deltas, so a residual,
// rounding rule or incomplete ending stays the caller's decision, and the
// chart draws exactly the levels it is given.
type WaterfallStep struct {
	ID    ID
	Label string
	Kind  StepKind
	// From and To are the levels before and after a delta step. A total step
	// uses To only. A missing level draws an empty placeholder and breaks the
	// connector.
	From, To Value
	// Text is the exact change or total as the caller formats it.
	Text   string
	Detail []string
	// Tone overrides the color that follows the direction of the step.
	Tone Tone
}

// entry and exit are the levels where a step joins its neighbors.
func (s WaterfallStep) entry() Value {
	if s.Kind == StepTotal {
		return s.To
	}
	return s.From
}

func (s WaterfallStep) exit() Value { return s.To }

// WaterfallSpec describes a waterfall: a beginning total, signed changes and an
// ending total with connectors between them.
type WaterfallSpec struct {
	Label, Summary string
	Steps          []WaterfallStep
	// FormatValue labels the level axis; display rounding only.
	FormatValue func(float64) string
	Selected    Selection
	Messages    Messages
	// MinColumn is the narrowest a column may be before the chart lays out as
	// rows. Zero uses 96dp.
	MinColumn unit.Dp
}

// Rows returns the text form of every step in order.
func (s WaterfallSpec) Rows() []TextRow {
	rows := make([]TextRow, 0, len(s.Steps))
	for _, st := range s.Steps {
		value, missing := valueText(st.valueForText(), st.Text, s.Messages)
		rows = append(rows, TextRow{
			ID: st.ID, Label: st.Label, Value: value, Detail: st.Detail, Missing: missing,
			Selected: s.Selected == Selection{Item: st.ID},
		})
	}
	return rows
}

// valueForText is the value whose presence decides whether the step has text
// of its own: both levels for a delta, the level for a total.
func (s WaterfallStep) valueForText() Value {
	if s.Kind == StepTotal {
		return s.To
	}
	if s.From.Valid() && s.To.Valid() {
		return Num(s.To.Number - s.From.Number)
	}
	return NoValue
}

// Validate lists empty and repeated step IDs.
func (s WaterfallSpec) Validate() []Problem {
	var check idCheck
	for _, st := range s.Steps {
		check.add("", st.ID)
	}
	return check.problems
}

// Breaks lists the steps whose entry level differs from the previous step's
// exit level. The chart draws a warning jump there and leaves the message and
// the arithmetic to the page: Wails shows an alert when the totals do not
// reconcile, and so can a page using this component. Steps with a missing
// level on either side of a join are not reported; they are drawn unconnected.
func (s WaterfallSpec) Breaks() []ID {
	var ids []ID
	for i := 1; i < len(s.Steps); i++ {
		exit, entry := s.Steps[i-1].exit(), s.Steps[i].entry()
		if exit.Valid() && entry.Valid() && !sameLevel(exit.Number, entry.Number) {
			ids = append(ids, s.Steps[i].ID)
		}
	}
	return ids
}

func sameLevel(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}

func (s WaterfallSpec) domain(maxTicks int) (ticks []float64, ok bool) {
	lo, hi := 0.0, 0.0
	for _, st := range s.Steps {
		for _, v := range []Value{st.From, st.To} {
			if st.Kind == StepTotal && v == st.From {
				continue
			}
			if v.Valid() {
				ok = true
				lo, hi = math.Min(lo, v.Number), math.Max(hi, v.Number)
			}
		}
	}
	if !ok {
		return nil, false
	}
	return niceTicks(lo, hi, maxTicks), true
}

// connector is a line joining two steps. warn marks a join whose levels differ.
type connector struct {
	from, to image.Point
	warn     bool
}

type waterfallStepPlan struct {
	sel Selection
	// cell is the hit area of the step.
	cell image.Rectangle
	// bar is empty when a level is missing; placeholder then marks the slot.
	// band is the strip a row's bar travels in, in the row layout.
	bar, placeholder, band image.Rectangle
	present                bool
	// up is true when the level rises or the total is positive. It is
	// geometry for the color only; ToneAuto reads it.
	up bool
	// label is the top-left of the label text; labelWidth wraps it.
	label      image.Point
	labelWidth int
	// amount is the top-left of the amount text in a row layout.
	amount image.Point
}

type waterfallPlan struct {
	// rows is true for the compact layout, where each step is a row.
	rows       bool
	plot       image.Rectangle
	level      scale
	ticks      []float64
	steps      []waterfallStepPlan
	connectors []connector
	order      []Selection
	height     int
}

// waterfallGeometry are the pixel inputs of a plan.
type waterfallGeometry struct {
	rows bool
	// width and plot describe the chart. In a row layout plot is the track
	// column shared by all rows.
	width int
	plot  image.Rectangle
	// labelTop is where column labels start; labelHeights are per step.
	labelTop int
	// cellBottom is the lower edge of the column cells.
	cellBottom int
	// rowHeights, headHeights, barHeight, rowGap and inset describe the rows.
	rowHeights, headHeights []int
	barHeight, rowGap       int
	// stubGap is the space between a row's label and its bar.
	stubGap int
	valueW  int
	inset   int
	// barFraction is the share of a column slot a bar fills.
	barFraction float64
	maxBarWidth int
	minBarWidth int
}

const minStepPx = 2

func (s WaterfallSpec) plan(g waterfallGeometry, ticks []float64) waterfallPlan {
	p := waterfallPlan{rows: g.rows, plot: g.plot, ticks: ticks}
	n := len(s.Steps)
	if len(ticks) > 0 {
		lo, hi := ticks[0], ticks[len(ticks)-1]
		if g.rows {
			p.level = scale{d0: lo, d1: hi, r0: float64(g.plot.Min.X), r1: float64(g.plot.Max.X)}
		} else {
			p.level = scale{d0: hi, d1: lo, r0: float64(g.plot.Min.Y), r1: float64(g.plot.Max.Y)}
		}
	}
	pos := func(v float64) int { return int(math.Round(p.level.at(v))) }
	y := 0
	for i, st := range s.Steps {
		sp := waterfallStepPlan{sel: Selection{Item: st.ID}}
		from := st.From
		if st.Kind == StepTotal {
			from = Num(0)
		}
		sp.present = from.Valid() && st.To.Valid()
		sp.up = st.To.Number >= from.Number
		var lo, hi int
		if sp.present {
			a, b := pos(from.Number), pos(st.To.Number)
			lo, hi = min(a, b), max(a, b)
			if hi-lo < minStepPx {
				mid := (lo + hi) / 2
				lo, hi = mid-minStepPx/2, mid+(minStepPx+1)/2
			}
		}
		if g.rows {
			h := g.rowHeights[i]
			sp.cell = image.Rect(0, y, g.width, y+h)
			sp.label = image.Pt(g.inset, y+g.rowGap)
			sp.labelWidth = g.width - 2*g.inset - g.valueW - g.rowGap
			sp.amount = image.Pt(g.width-g.inset-g.valueW, y+g.rowGap)
			barY := y + h - g.rowGap - g.barHeight
			sp.band = image.Rect(g.plot.Min.X, barY, g.plot.Max.X, barY+g.barHeight)
			if sp.present {
				sp.bar = image.Rect(lo, barY, hi, barY+g.barHeight)
			} else {
				zero := pos(0)
				sp.placeholder = image.Rect(zero-g.barHeight/2, barY, zero+g.barHeight/2, barY+g.barHeight)
			}
			y += h
		} else {
			slot := float64(g.plot.Dx()) / float64(max(1, n))
			x0 := g.plot.Min.X + int(math.Round(slot*float64(i)))
			x1 := g.plot.Min.X + int(math.Round(slot*float64(i+1)))
			sp.cell = image.Rect(x0, g.plot.Min.Y, x1, g.cellBottom)
			barW := min(max(int(float64(x1-x0)*g.barFraction), g.minBarWidth), g.maxBarWidth)
			barW = min(barW, x1-x0)
			center := (x0 + x1) / 2
			if sp.present {
				sp.bar = image.Rect(center-barW/2, lo, center-barW/2+barW, hi)
			} else {
				zero := pos(0)
				sp.placeholder = image.Rect(center-barW/2, zero-g.minBarWidth/2, center-barW/2+barW, zero+g.minBarWidth/2)
			}
			sp.label = image.Pt(x0+g.inset, g.labelTop)
			sp.labelWidth = max(1, x1-x0-2*g.inset)
		}
		p.steps = append(p.steps, sp)
		p.order = append(p.order, sp.sel)
	}
	p.height = y
	// Connectors join the exit level of a step to the entry level of the next.
	// A join needs a drawn bar on both sides; a step with a missing level is
	// left unconnected.
	for i := 0; i+1 < n; i++ {
		exit, entry := s.Steps[i].exit(), s.Steps[i+1].entry()
		a, b := p.steps[i], p.steps[i+1]
		if !exit.Valid() || !entry.Valid() || !a.present || !b.present {
			continue
		}
		warn := !sameLevel(exit.Number, entry.Number)
		if g.rows {
			// Each row carries its label between the two bars, so the join is
			// two short guides at the shared level instead of one line: one
			// leaves the bottom of the upper bar and one reaches the top of
			// the lower bar.
			x, nx := pos(exit.Number), pos(entry.Number)
			p.connectors = append(p.connectors,
				connector{from: image.Pt(x, a.bar.Max.Y), to: image.Pt(x, b.label.Y-g.rowGap/2), warn: warn},
				connector{from: image.Pt(nx, b.bar.Min.Y-g.stubGap), to: image.Pt(nx, b.bar.Min.Y), warn: warn})
			continue
		}
		yExit := pos(exit.Number)
		p.connectors = append(p.connectors, connector{from: image.Pt(a.bar.Max.X, yExit), to: image.Pt(b.bar.Min.X, yExit)})
		if warn {
			p.connectors = append(p.connectors, connector{from: image.Pt(b.bar.Min.X, yExit), to: image.Pt(b.bar.Min.X, pos(entry.Number)), warn: true})
		}
	}
	return p
}

func (p waterfallPlan) pick(at image.Point) (Selection, bool) {
	for _, s := range p.steps {
		if at.In(s.cell) {
			return s.sel, true
		}
	}
	return Selection{}, false
}

func (p waterfallPlan) stepFor(sel Selection) (waterfallStepPlan, bool) {
	for _, s := range p.steps {
		if s.sel == sel {
			return s, true
		}
	}
	return waterfallStepPlan{}, false
}

// WaterfallChart is the widget state of one waterfall.
type WaterfallChart struct {
	interaction
}

const defaultMinColumn unit.Dp = 96

// Layout draws the chart at the width of gtx.Constraints.Max and returns the
// selection requests made this frame. It lays out columns when every column can
// be at least MinColumn wide and rows otherwise, so narrow windows stay legible.
func (c *WaterfallChart) Layout(gtx layout.Context, view ui.View, spec WaterfallSpec) (layout.Dimensions, []Event) {
	width := gtx.Constraints.Max.X
	if len(spec.Steps) == 0 {
		return drawNotice(gtx, view, image.Pt(width, gtx.Dp(160)), spec.Messages.Empty), nil
	}
	pad := gtx.Dp(ringPad)
	inner := max(1, width-2*pad)
	colors := view.Colors
	ticks, ok := spec.domain(6)
	if !ok {
		return drawNotice(gtx, view, image.Pt(width, gtx.Dp(160)), spec.Messages.Missing), nil
	}
	label := func(s string) textBlock {
		return textBlock{style: ui.TextCaption, content: s, ink: colors.SecondaryInk, maxWidth: inner, maxLines: 1}
	}
	yLabelW := 0
	for _, t := range ticks {
		yLabelW = max(yLabelW, label(spec.formatValue(t)).measure(gtx, view).X)
	}
	gap := gtx.Dp(ui.Space8)
	minColumn := spec.MinColumn
	if minColumn == 0 {
		minColumn = defaultMinColumn
	}
	plotLeft := yLabelW + gap
	texts := make([]string, len(spec.Steps))
	amountW := 0
	for i, st := range spec.Steps {
		texts[i], _ = valueText(st.valueForText(), st.Text, spec.Messages)
		amountW = max(amountW, textBlock{style: ui.TextCaptionStrong, content: texts[i]}.natural(gtx, view))
	}
	// A column also has to hold its longest amount on one line.
	columnsFit := (inner-plotLeft)/len(spec.Steps) >= max(gtx.Dp(minColumn), amountW+2*gtx.Dp(ui.Space8))
	g := waterfallGeometry{rows: !columnsFit, stubGap: gap, width: inner, inset: gtx.Dp(ui.Space8), rowGap: gtx.Dp(ui.Space8), barHeight: gtx.Dp(barRowHeight),
		barFraction: 0.6, maxBarWidth: gtx.Dp(56), minBarWidth: gtx.Dp(12)}
	var chartHeight int
	if g.rows {
		valueW := 0
		for _, t := range texts {
			valueW = max(valueW, textBlock{style: ui.TextLabel, content: t, maxWidth: inner, maxLines: 1}.measure(gtx, view).X)
		}
		g.valueW = min(valueW, inner/2)
		g.plot = image.Rect(g.inset, 0, inner-g.inset, 0)
		for i, st := range spec.Steps {
			labelH := textBlock{style: ui.TextBody, content: st.Label, maxWidth: max(1, inner-2*g.inset-g.valueW-g.rowGap)}.measure(gtx, view).Y
			valueH := textBlock{style: ui.TextLabel, content: texts[i], maxWidth: max(1, g.valueW)}.measure(gtx, view).Y
			head := max(labelH, valueH)
			g.headHeights = append(g.headHeights, head)
			g.rowHeights = append(g.rowHeights, head+g.barHeight+gap+2*g.rowGap)
		}
		chartHeight = 0
		for _, h := range g.rowHeights {
			chartHeight += h
		}
	} else {
		slot := (inner - plotLeft) / len(spec.Steps)
		labelH, valueH := 0, 0
		for i, st := range spec.Steps {
			labelH = max(labelH, textBlock{style: ui.TextCaption, content: st.Label, maxWidth: max(1, slot-2*g.inset)}.measure(gtx, view).Y)
			valueH = max(valueH, textBlock{style: ui.TextCaptionStrong, content: texts[i], maxWidth: max(1, slot-2*g.inset)}.measure(gtx, view).Y)
		}
		plotTop, plotHeight := gtx.Dp(ui.Space8), gtx.Dp(200)
		g.plot = image.Rect(plotLeft, plotTop, inner, plotTop+plotHeight)
		g.labelTop = g.plot.Max.Y + gap
		g.cellBottom = g.labelTop + labelH + gtx.Dp(ui.Space4) + valueH + gap
		g.headHeights = []int{labelH, valueH}
		chartHeight = g.cellBottom
	}
	plan := spec.plan(g, ticks)
	if g.rows {
		chartHeight = plan.height
	}

	items := frameItems{order: plan.order, selected: spec.Selected, pick: plan.pick}
	events := c.update(gtx, c, items)
	active := c.active(gtx, c, plan.order)

	dims := accessibility.List(gtx, c, spec.Label, func(gtx layout.Context) layout.Dimensions {
		defer op.Offset(image.Pt(pad, pad)).Push(gtx.Ops).Pop()
		hovering := !c.hover.IsZero()
		y := chartHeight
		if plan.rows {
			c.drawRows(gtx, view, spec, plan, texts, active)
			y += c.drawRowScale(gtx, view, spec, plan, y)
		} else {
			c.drawColumns(gtx, view, spec, plan, texts, active, yLabelW)
		}
		y += gtx.Dp(ui.Space8)
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

func (s WaterfallSpec) formatValue(v float64) string {
	if s.FormatValue != nil {
		return s.FormatValue(v)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func (s WaterfallSpec) readout(sel Selection) *readout {
	if sel.IsZero() {
		return nil
	}
	for _, st := range s.Steps {
		if st.ID == sel.Item {
			value, missing := valueText(st.valueForText(), st.Text, s.Messages)
			return &readout{label: st.Label, value: value, detail: st.Detail, selected: sel == s.Selected, missing: missing}
		}
	}
	return nil
}

// drawBar paints a step's bar, or an outlined placeholder where a level is
// missing. A selected bar also carries an ink outline.
func drawBar(gtx layout.Context, view ui.View, st WaterfallStep, sp waterfallStepPlan, selected bool) {
	colors := view.Colors
	if !sp.present {
		if !sp.placeholder.Empty() {
			ui.StrokeRoundRect(gtx, sp.placeholder, ui.RadiusSM, 1, colors.SecondaryInk)
		}
		return
	}
	tone := st.Tone
	fill := colors.Accent
	switch {
	case tone != ToneAuto:
		fill = toneColor(colors, tone, 0)
	case st.Kind == StepDelta && sp.up:
		fill = colors.Success
	case st.Kind == StepDelta:
		fill = colors.Destructive
	}
	ui.FillRoundRect(gtx, sp.bar, ui.RadiusSM, fill)
	if selected {
		ui.StrokeRoundRect(gtx, sp.bar.Inset(-gtx.Dp(3)), ui.RadiusSM, 2, colors.Ink)
	}
}

func (c *WaterfallChart) drawColumns(gtx layout.Context, view ui.View, spec WaterfallSpec, plan waterfallPlan, texts []string, active Selection, yLabelW int) {
	colors := view.Colors
	plot := plan.plot
	gap := gtx.Dp(ui.Space8)
	for _, tick := range plan.ticks {
		py := int(math.Round(plan.level.at(tick)))
		ink, thickness := colors.Divider, 1
		if tick == 0 {
			ink, thickness = ui.WithAlpha(colors.Ink, 0.4), max(1, gtx.Dp(1)+gtx.Dp(1)/2)
		}
		hairline(gtx, image.Pt(plot.Min.X, py), image.Pt(plot.Max.X, py), thickness, ink)
		block := textBlock{style: ui.TextCaption, content: spec.formatValue(tick), ink: colors.SecondaryInk, maxWidth: yLabelW, maxLines: 1, align: text.End}
		size := block.measure(gtx, view)
		block.draw(gtx, view, image.Pt(plot.Min.X-gap-yLabelW, py-size.Y/2))
	}
	for i, sp := range plan.steps {
		st := spec.Steps[i]
		selected := sp.sel == spec.Selected
		c.drawCell(gtx, view, sp, selected, sp.sel == active)
		drawBar(gtx, view, st, sp, selected)
		labelInk := colors.SecondaryInk
		weight := ui.TextCaption
		if st.Kind == StepTotal {
			labelInk, weight = colors.Ink, ui.TextCaptionStrong
		}
		size := textBlock{style: weight, content: st.Label, ink: labelInk, maxWidth: sp.labelWidth, align: text.Middle}.draw(gtx, view, sp.label)
		valueInk := colors.Ink
		if !sp.present {
			valueInk = colors.SecondaryInk
		}
		textBlock{style: ui.TextCaptionStrong, content: texts[i], ink: valueInk, maxWidth: sp.labelWidth, align: text.Middle}.draw(gtx, view, image.Pt(sp.label.X, sp.label.Y+size.Y+gtx.Dp(ui.Space4)))
		if label := spec.readout(sp.sel); label != nil {
			semanticItem(gtx, sp.cell, itemLabel(*label, spec.Messages))
		}
	}
	c.drawConnectors(gtx, view, plan)
	if sp, ok := plan.stepFor(active); ok && (!c.hover.IsZero() || gtx.Focused(c)) && sp.present {
		if item := spec.readout(active); item != nil {
			anchor := image.Pt((sp.bar.Min.X+sp.bar.Max.X)/2, sp.bar.Min.Y)
			drawTooltip(gtx, view, image.Rectangle{Max: image.Pt(plot.Max.X, sp.cell.Max.Y)}, anchor, *item)
		}
	}
}

func (c *WaterfallChart) drawConnectors(gtx layout.Context, view ui.View, plan waterfallPlan) {
	colors := view.Colors
	w := max(1, gtx.Dp(1))
	for _, link := range plan.connectors {
		ink := ui.WithAlpha(colors.Ink, 0.45)
		if link.warn {
			ink = colors.Warning
		}
		r := image.Rectangle{Min: link.from, Max: link.to}.Canon()
		if r.Dx() == 0 {
			r.Max.X = r.Min.X + w
		}
		if r.Dy() == 0 {
			r.Max.Y = r.Min.Y + w
		}
		if link.warn {
			// A break is drawn heavier and dashed so it cannot pass for a join.
			w2 := max(3, gtx.Dp(3))
			if r.Dx() <= w2 {
				for y := r.Min.Y; y < r.Max.Y; y += 2 * gtx.Dp(3) {
					fillRect(gtx, image.Rect(r.Min.X, y, r.Min.X+w2, min(y+gtx.Dp(3), r.Max.Y)), ink)
				}
			} else {
				for x := r.Min.X; x < r.Max.X; x += 2 * gtx.Dp(3) {
					fillRect(gtx, image.Rect(x, r.Min.Y, min(x+gtx.Dp(3), r.Max.X), r.Min.Y+w2), ink)
				}
			}
			continue
		}
		fillRect(gtx, r, ink)
	}
}

// drawCell paints the hover, selection and focus states behind a step.
func (c *WaterfallChart) drawCell(gtx layout.Context, view ui.View, sp waterfallStepPlan, selected, active bool) {
	colors := view.Colors
	switch {
	case selected:
		ui.FillRoundRect(gtx, sp.cell, ui.RadiusSM, colors.Tint(colors.Accent, 0.10))
		rail := image.Rect(sp.cell.Min.X+gtx.Dp(ui.Space8), sp.cell.Max.Y-gtx.Dp(3), sp.cell.Max.X-gtx.Dp(ui.Space8), sp.cell.Max.Y)
		ui.FillRoundRect(gtx, rail, ui.RadiusFull, colors.Accent)
	case active:
		ui.FillRoundRect(gtx, sp.cell, ui.RadiusSM, colors.HoverSurface)
	}
	if active && gtx.Focused(c) {
		ui.StrokeRoundRect(gtx, sp.cell, ui.RadiusSM, 2, ui.WithAlpha(colors.Accent, 0.8))
	}
}

func (c *WaterfallChart) drawRows(gtx layout.Context, view ui.View, spec WaterfallSpec, plan waterfallPlan, texts []string, active Selection) {
	colors := view.Colors
	zeroX := int(math.Round(plan.level.at(0)))
	for i, sp := range plan.steps {
		selected := sp.sel == spec.Selected
		c.drawCell(gtx, view, sp, selected, sp.sel == active)
		if i > 0 && !selected {
			hairline(gtx, image.Pt(sp.cell.Min.X, sp.cell.Min.Y), image.Pt(sp.cell.Max.X, sp.cell.Min.Y), 1, ui.WithAlpha(colors.Divider, 0.6))
		}
	}
	// The label sits above each bar at the track's left edge, so the zero axis
	// is drawn only beside the bars.
	for _, sp := range plan.steps {
		pad := gtx.Dp(ui.Space4)
		hairline(gtx, image.Pt(zeroX, sp.band.Min.Y-pad), image.Pt(zeroX, sp.band.Max.Y+pad), 1, ui.WithAlpha(colors.Ink, 0.3))
	}
	for i, sp := range plan.steps {
		st := spec.Steps[i]
		selected := sp.sel == spec.Selected
		weight := ui.TextBody
		if st.Kind == StepTotal {
			weight = ui.TextLabel
		}
		textBlock{style: weight, content: st.Label, ink: colors.Ink, maxWidth: max(1, sp.labelWidth)}.draw(gtx, view, sp.label)
		valueInk := colors.Ink
		if !sp.present {
			valueInk = colors.SecondaryInk
		}
		valueW := sp.cell.Max.X - sp.amount.X - gtx.Dp(ui.Space8)
		textBlock{style: ui.TextLabel, content: texts[i], ink: valueInk, maxWidth: max(1, valueW), align: text.End}.draw(gtx, view, sp.amount)
		drawBar(gtx, view, st, sp, selected)
		if item := spec.readout(sp.sel); item != nil {
			semanticItem(gtx, sp.cell, itemLabel(*item, spec.Messages))
		}
	}
	c.drawConnectors(gtx, view, plan)
}

func (c *WaterfallChart) drawRowScale(gtx layout.Context, view ui.View, spec WaterfallSpec, plan waterfallPlan, y int) int {
	colors := view.Colors
	gap := gtx.Dp(ui.Space4)
	height := 0
	first, last := plan.ticks[0], plan.ticks[len(plan.ticks)-1]
	for _, m := range []struct {
		at    float64
		align text.Alignment
	}{{first, text.Start}, {0, text.Middle}, {last, text.End}} {
		if m.at == 0 && m.align != text.Middle || m.align == text.Middle && (first >= 0 || last <= 0) {
			continue
		}
		block := textBlock{style: ui.TextCaption, content: spec.formatValue(m.at), ink: colors.SecondaryInk, maxWidth: plan.plot.Dx(), maxLines: 1}
		size := block.measure(gtx, view)
		x := int(math.Round(plan.level.at(m.at)))
		switch m.align {
		case text.Middle:
			x -= size.X / 2
		case text.End:
			x -= size.X
		}
		x = max(plan.plot.Min.X, min(x, plan.plot.Max.X-size.X))
		block.draw(gtx, view, image.Pt(x, y+gap))
		height = max(height, size.Y+gap)
	}
	return height
}
