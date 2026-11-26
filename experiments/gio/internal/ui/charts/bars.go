package charts

import (
	"image"
	"image/color"
	"math"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

// Bar is one row of a signed bar chart, such as a contribution of one
// instrument to a return.
type Bar struct {
	// ID is stable across refreshes.
	ID ID
	// Label is the full name. It wraps in the label column and is never
	// truncated.
	Label string
	// Value positions the bar relative to the shared zero axis. A missing value
	// draws no bar; a real zero draws a tick on the axis.
	Value Value
	// Text is the exact signed amount as the caller formats it.
	Text   string
	Detail []string
	// Tone overrides the color that follows the sign of Value.
	Tone Tone
}

// BarSpec describes a horizontal signed bar chart. Bars appear in the order
// given. The chart neither sorts nor limits them; ranking and "show top N"
// belong to the owner of the read model.
type BarSpec struct {
	Label, Summary string
	Bars           []Bar
	// Symmetric makes the zero axis the center, so equal magnitudes have equal
	// lengths on both sides. By default the axis sits where zero falls between
	// the smallest and largest value, which uses the full width.
	Symmetric bool
	// ActionableRows exposes each row as a native accessibility action while
	// the chart still owns pointer and keyboard input.
	ActionableRows bool
	// FormatValue labels the ends of the scale under the bars. Without it no
	// scale labels are drawn.
	FormatValue func(float64) string
	Selected    Selection
	Messages    Messages
}

// Rows returns the text form of every bar in order.
func (s BarSpec) Rows() []TextRow {
	rows := make([]TextRow, 0, len(s.Bars))
	for _, b := range s.Bars {
		value, missing := valueText(b.Value, b.Text, s.Messages)
		rows = append(rows, TextRow{
			ID: b.ID, Label: b.Label, Value: value, Detail: b.Detail, Missing: missing,
			Selected: s.Selected == Selection{Item: b.ID},
		})
	}
	return rows
}

// Validate lists empty and repeated bar IDs. Charts do not call it; pages and
// tests do, because a duplicate would make two bars select together.
func (s BarSpec) Validate() []Problem {
	var check idCheck
	for _, b := range s.Bars {
		check.add("", b.ID)
	}
	return check.problems
}

// domain is the signed extent of the scale. ok is false when no bar has a
// value; the scale is then a placeholder.
func (s BarSpec) domain() (lo, hi float64, ok bool) {
	for _, b := range s.Bars {
		if !b.Value.Valid() {
			continue
		}
		ok = true
		lo, hi = math.Min(lo, b.Value.Number), math.Max(hi, b.Value.Number)
	}
	if s.Symmetric {
		m := math.Max(-lo, hi)
		lo, hi = -m, m
	}
	if lo == 0 && hi == 0 {
		lo, hi = -1, 1
	}
	return lo, hi, ok
}

// barGeometry are the pixel inputs of a bar plan.
type barGeometry struct {
	width int
	// wide places the label, bar and amount in one line; otherwise the label
	// and amount share a line and the bar sits below.
	wide bool
	// labelW and valueW are the label and amount columns of a wide row. In a
	// narrow row only valueW is used, for the amount at the trailing edge.
	labelW, valueW, gap int
	// inset keeps text and bars off the row edges, where the selection rail
	// and the focus outline are drawn.
	inset int
	// rowHeights are the measured heights of the rows, label text included.
	rowHeights []int
	barHeight  int
	// textHeights are the line heights used to place the bar of a narrow row.
	headHeights []int
	rowGap      int
	// ticks are the value axis ticks. When set, the scale spans the first to
	// the last tick, so a gridline falls on every round value.
	ticks []float64
}

type barRowPlan struct {
	sel Selection
	// row is the whole row in chart coordinates.
	row image.Rectangle
	// band is the horizontal strip the bar travels in; bar is empty for a
	// missing value.
	band, bar image.Rectangle
	present   bool
	// value is the signed number, for the color.
	value float64
	// label and amount are the top-left corners of the texts. For a wide row
	// the amount is right-aligned in valueW.
	label, amount image.Point
	labelWidth    int
}

type barPlan struct {
	rows []barRowPlan
	// zeroX is the zero axis in chart coordinates.
	zeroX              int
	trackMin, trackMax int
	lo, hi             float64
	height             int
	scale              scale
	ticks              []float64
	order              []Selection
	// compact is true for the two-line row layout, where the label shares the
	// row's left edge with the zero axis.
	compact bool
}

// minBarTickPx keeps zero and subpixel bars visible.
const minBarTickPx = 3

func (s BarSpec) plan(g barGeometry) barPlan {
	lo, hi, _ := s.domain()
	if len(g.ticks) >= 2 {
		lo, hi = g.ticks[0], g.ticks[len(g.ticks)-1]
	}
	p := barPlan{lo: lo, hi: hi, compact: !g.wide, ticks: g.ticks}
	if g.wide {
		p.trackMin, p.trackMax = g.inset+g.labelW+g.gap, g.width-g.inset-g.valueW-g.gap
	} else {
		p.trackMin, p.trackMax = g.inset, g.width-g.inset
	}
	p.trackMax = max(p.trackMax, p.trackMin+1)
	p.scale = scale{d0: lo, d1: hi, r0: float64(p.trackMin), r1: float64(p.trackMax)}
	p.zeroX = int(math.Round(p.scale.at(0)))
	y := 0
	for i, b := range s.Bars {
		h := g.rowHeights[i]
		row := barRowPlan{sel: Selection{Item: b.ID}, row: image.Rect(0, y, g.width, y+h), present: b.Value.Valid(), value: b.Value.Number}
		barY0 := y + (h-g.barHeight)/2
		if g.wide {
			row.label = image.Pt(g.inset, y+(h-g.headHeights[i])/2)
			row.labelWidth = g.labelW
			row.amount = image.Pt(g.width-g.inset-g.valueW, y+(h-g.headHeights[i])/2)
		} else {
			row.label = image.Pt(g.inset, y+g.rowGap)
			row.labelWidth = g.width - 2*g.inset - g.valueW - g.gap
			row.amount = image.Pt(g.width-g.inset-g.valueW, y+g.rowGap)
			barY0 = y + h - g.rowGap - g.barHeight
		}
		row.band = image.Rect(p.trackMin, barY0, p.trackMax, barY0+g.barHeight)
		if row.present {
			x := int(math.Round(p.scale.at(b.Value.Number)))
			x0, x1 := min(x, p.zeroX), max(x, p.zeroX)
			if x1-x0 < minBarTickPx {
				switch {
				case b.Value.Number < 0:
					x0 = max(p.trackMin, p.zeroX-minBarTickPx)
				case b.Value.Number > 0:
					x1 = min(p.trackMax, p.zeroX+minBarTickPx)
				default:
					x0, x1 = p.zeroX-minBarTickPx/2, p.zeroX+(minBarTickPx+1)/2
				}
			}
			row.bar = image.Rect(x0, barY0, x1, barY0+g.barHeight)
		}
		p.rows = append(p.rows, row)
		p.order = append(p.order, row.sel)
		y += h
	}
	p.height = y
	return p
}

func (p barPlan) pick(at image.Point) (Selection, bool) {
	for _, r := range p.rows {
		if at.In(r.row) {
			return r.sel, true
		}
	}
	return Selection{}, false
}

func (p barPlan) rowFor(sel Selection) (barRowPlan, bool) {
	for _, r := range p.rows {
		if r.sel == sel {
			return r, true
		}
	}
	return barRowPlan{}, false
}

func toneColor(c *ui.Palette, tone Tone, sign float64) color.NRGBA {
	switch tone {
	case ToneGain:
		return c.Success
	case ToneLoss:
		return c.Destructive
	case ToneNeutral:
		return ui.Mix(c.Surface, c.Ink, 0.55)
	case ToneWarning:
		return c.Warning
	}
	switch {
	case sign > 0:
		return c.Success
	case sign < 0:
		return c.Destructive
	}
	return ui.Mix(c.Surface, c.Ink, 0.55)
}

const (
	// rowsWideBreakpoint is the content width from which a bar row puts its
	// label, bar and amount on one line.
	rowsWideBreakpoint unit.Dp = 480
	barRowHeight       unit.Dp = 14
	barRowPad          unit.Dp = 10
)

// BarChart is the widget state of one bar chart.
type BarChart struct {
	interaction
	nativeRows map[ID]*widget.Clickable
}

// Layout draws the chart at the width of gtx.Constraints.Max and returns the
// selection requests made this frame.
func (c *BarChart) Layout(gtx layout.Context, view ui.View, spec BarSpec) (layout.Dimensions, []Event) {
	width := gtx.Constraints.Max.X
	colors := view.Colors
	if len(spec.Bars) == 0 {
		c.nativeRows = nil
		return drawNotice(gtx, view, image.Pt(width, gtx.Dp(120)), spec.Messages.Empty), nil
	}
	pad := gtx.Dp(ringPad)
	inner := max(1, width-2*pad)
	wide := ui.WidthAtLeast(gtx, inner, rowsWideBreakpoint)
	gap := gtx.Dp(ui.Space12)

	// Amount texts decide the amount column; labels then wrap in what is left.
	texts := make([]string, len(spec.Bars))
	valueW := 0
	for i, b := range spec.Bars {
		texts[i], _ = valueText(b.Value, b.Text, spec.Messages)
		valueW = max(valueW, textBlock{style: ui.TextLabel, content: texts[i], maxWidth: inner, maxLines: 1}.measure(gtx, view).X)
	}
	valueW = min(valueW, inner/2)
	inset := gtx.Dp(ui.Space12)
	labelW := min(max(inner*3/10, gtx.Dp(120)), gtx.Dp(260))
	if wide && inner-2*inset-labelW-valueW-2*gap < gtx.Dp(80) {
		labelW = max(gtx.Dp(80), inner-2*inset-valueW-2*gap-gtx.Dp(80))
	}
	g := barGeometry{
		width: inner, wide: wide, labelW: labelW, valueW: valueW, gap: gap, inset: inset,
		barHeight: gtx.Dp(barRowHeight), rowGap: gtx.Dp(ui.Space8),
	}
	if spec.FormatValue != nil {
		trackW := inner - 2*inset
		if wide {
			trackW -= labelW + valueW + 2*gap
		}
		g.ticks = spec.axisTicks(gtx, view, trackW)
	}
	narrowLabel := inner - 2*inset - valueW - gap
	for i, b := range spec.Bars {
		labelWidth := narrowLabel
		if wide {
			labelWidth = labelW
		}
		labelH := textBlock{style: ui.TextBody, content: b.Label, maxWidth: max(1, labelWidth)}.measure(gtx, view).Y
		valueH := textBlock{style: ui.TextLabel, content: texts[i], maxWidth: max(1, valueW)}.measure(gtx, view).Y
		head := max(labelH, valueH)
		g.headHeights = append(g.headHeights, head)
		if wide {
			g.rowHeights = append(g.rowHeights, max(head, g.barHeight)+2*gtx.Dp(barRowPad))
		} else {
			g.rowHeights = append(g.rowHeights, head+g.barHeight+gtx.Dp(ui.Space8)+2*g.rowGap)
		}
	}
	plan := spec.plan(g)

	items := frameItems{order: plan.order, selected: spec.Selected, pick: plan.pick}
	if spec.ActionableRows {
		items.activate = c.nativePress(gtx, plan.order)
	} else {
		c.nativeRows = nil
	}
	events := c.update(gtx, c, items)
	active := c.active(gtx, c, plan.order)

	dims := accessibility.List(gtx, c, spec.Label, func(gtx layout.Context) layout.Dimensions {
		defer op.Offset(image.Pt(pad, pad)).Push(gtx.Ops).Pop()
		hovering := !c.hover.IsZero()
		c.drawRows(gtx, view, spec, plan, texts, active)
		y := plan.height
		if spec.FormatValue != nil {
			y += c.drawScale(gtx, view, spec, plan, y)
		}
		y += gtx.Dp(ui.Space8)
		rgtx := gtx
		rgtx.Constraints.Max.X = inner
		shown := active
		if shown.IsZero() {
			shown = spec.Selected
		}
		offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		readout := spec.readout(shown)
		drawReadout := func(gtx layout.Context) layout.Dimensions {
			return layoutReadout(gtx, view, spec.Messages, spec.Summary, readout)
		}
		var read layout.Dimensions
		if spec.ActionableRows && readout != nil {
			// The focused row already exposes the same text and its action.
			read = accessibility.Hidden(rgtx, drawReadout)
		} else {
			read = drawReadout(rgtx)
		}
		offset.Pop()
		y += read.Size.Y

		size := image.Pt(width, y+2*pad)
		// Input covers the padded chart; row positions are offset by pad.
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

func (c *BarChart) nativePress(gtx layout.Context, order []Selection) Selection {
	if c.nativeRows == nil {
		c.nativeRows = make(map[ID]*widget.Clickable, len(order))
	}
	var pressed Selection
	for _, sel := range order {
		button := c.nativeRows[sel.Item]
		if button == nil {
			button = new(widget.Clickable)
			c.nativeRows[sel.Item] = button
		}
		// These clickables are activated by the native bridge. They have no
		// input operations of their own, so the chart remains one Tab stop.
		if button.Clicked(gtx) && gtx.Enabled() {
			pressed = sel
		}
	}
	if len(c.nativeRows) > len(order) {
		for id := range c.nativeRows {
			if !contains(order, Selection{Item: id}) {
				delete(c.nativeRows, id)
			}
		}
	}
	return pressed
}

// axisTicks picks round value ticks that fit the track with their labels
// side by side. The end labels only estimate the label width, so the chosen
// ticks are measured and the budget shrinks while they would collide.
func (s BarSpec) axisTicks(gtx layout.Context, view ui.View, trackW int) []float64 {
	lo, hi, _ := s.domain()
	widest := func(ticks []float64) int {
		w := 0
		for _, t := range ticks {
			w = max(w, textBlock{style: ui.TextCaption, content: s.FormatValue(t)}.natural(gtx, view))
		}
		return w
	}
	space := gtx.Dp(ui.Space16)
	labelW := widest([]float64{lo, hi})
	ticks := niceTicks(lo, hi, 2)
	for budget := min(8, max(2, trackW/max(1, labelW+space))); budget >= 2; budget-- {
		ticks = niceTicks(lo, hi, budget)
		if (len(ticks)-1)*(widest(ticks)+space) <= trackW || budget == 2 {
			break
		}
	}
	return ticks
}

func (s BarSpec) readout(sel Selection) *readout {
	if sel.IsZero() {
		return nil
	}
	for _, b := range s.Bars {
		if b.ID == sel.Item {
			value, missing := valueText(b.Value, b.Text, s.Messages)
			return &readout{label: b.Label, value: value, detail: b.Detail, selected: sel == s.Selected, missing: missing}
		}
	}
	return nil
}

func (c *BarChart) drawRows(gtx layout.Context, view ui.View, spec BarSpec, plan barPlan, texts []string, active Selection) {
	colors := view.Colors
	radius := ui.RadiusSM
	for i, row := range plan.rows {
		selected := row.sel == spec.Selected
		if selected {
			ui.FillRoundRect(gtx, row.row, radius, colors.Tint(colors.Accent, 0.10))
			ui.FillRoundRect(gtx, image.Rect(row.row.Min.X, row.row.Min.Y+gtx.Dp(4), row.row.Min.X+gtx.Dp(3), row.row.Max.Y-gtx.Dp(4)), ui.RadiusFull, colors.Accent)
		} else if row.sel == active {
			ui.FillRoundRect(gtx, row.row, radius, colors.HoverSurface)
		}
		if i > 0 && !selected {
			hairline(gtx, image.Pt(row.row.Min.X, row.row.Min.Y), image.Pt(row.row.Max.X, row.row.Min.Y), 1, ui.WithAlpha(colors.Divider, 0.6))
		}
	}
	// The shared zero axis runs behind every bar. In the compact layout the
	// label sits above the bar at the same left edge, so the axis is drawn only
	// beside the bars.
	// Value gridlines sit on the round ticks, quieter than the zero axis.
	axis := ui.WithAlpha(colors.Ink, 0.3)
	grid := ui.WithAlpha(colors.Divider, 0.9)
	vertical := func(x int, ink color.NRGBA) {
		if plan.compact {
			pad := gtx.Dp(ui.Space4)
			for _, row := range plan.rows {
				hairline(gtx, image.Pt(x, row.band.Min.Y-pad), image.Pt(x, row.band.Max.Y+pad), 1, ink)
			}
			return
		}
		hairline(gtx, image.Pt(x, 0), image.Pt(x, plan.height), 1, ink)
	}
	for _, tick := range plan.ticks {
		if x := int(math.Round(plan.scale.at(tick))); tick != 0 && x != plan.zeroX {
			vertical(x, grid)
		}
	}
	vertical(plan.zeroX, axis)
	cursor := c.cursor
	if !contains(plan.order, cursor) {
		cursor = defaultCursor(frameItems{order: plan.order, selected: spec.Selected})
	}
	for i, row := range plan.rows {
		b := spec.Bars[i]
		c.drawRowLabels(gtx, view, spec, row, b, texts[i], row.sel == cursor)
		if row.present {
			tone := toneColor(colors, b.Tone, row.value)
			if row.sel != active && !active.IsZero() && row.sel != spec.Selected {
				tone = ui.Mix(colors.Surface, tone, 0.55)
			}
			ui.FillRoundRect(gtx, row.bar, ui.Space4, tone)
		}
		if row.sel == active && gtx.Focused(c) {
			ui.StrokeRoundRect(gtx, row.row, radius, 2, ui.WithAlpha(colors.Accent, 0.8))
		}
	}
}

func (c *BarChart) drawRowLabels(gtx layout.Context, view ui.View, spec BarSpec, row barRowPlan, b Bar, amount string, active bool) {
	defer op.Offset(row.row.Min).Push(gtx.Ops).Pop()
	content := func(gtx layout.Context) layout.Dimensions {
		colors := view.Colors
		textBlock{style: ui.TextBody, content: b.Label, ink: colors.Ink, maxWidth: max(1, row.labelWidth)}.draw(gtx, view, row.label.Sub(row.row.Min))
		ink := colors.Ink
		if !row.present {
			ink = colors.SecondaryInk
		}
		valueW := row.row.Max.X - row.amount.X - gtx.Dp(ui.Space12)
		textBlock{style: ui.TextLabel, content: amount, ink: ink, maxWidth: max(1, valueW), align: text.End}.draw(gtx, view, row.amount.Sub(row.row.Min))
		return layout.Dimensions{Size: row.row.Size()}
	}
	selected := row.sel == spec.Selected
	label := itemLabel(readout{label: b.Label, value: amount, detail: b.Detail, selected: selected}, spec.Messages)
	if spec.ActionableRows {
		focus := accessibility.OptionFocus{Owner: c, Active: active, Move: func() {
			c.cursor, c.hover = row.sel, Selection{}
		}}
		accessibility.Option(gtx, c.nativeRows[b.ID], label, selected, focus, func(gtx layout.Context) layout.Dimensions {
			semantic.LabelOp(label).Add(gtx.Ops)
			return content(gtx)
		})
		return
	}
	accessibility.Summary(gtx, label, content)
}

// drawScale labels the ends of the scale and zero under the bars and returns
// its height.
func (c *BarChart) drawScale(gtx layout.Context, view ui.View, spec BarSpec, plan barPlan, y int) int {
	colors := view.Colors
	gap := gtx.Dp(ui.Space6)
	ticks := plan.ticks
	if len(ticks) < 2 {
		ticks = []float64{plan.lo, plan.hi}
	}
	// Labels center on their ticks, shift inward at the ends of the track and
	// are skipped where they would touch a neighbor, zero always kept.
	type placed struct{ x0, x1 int }
	var shown []placed
	fits := func(p placed) bool {
		for _, q := range shown {
			if p.x0 < q.x1+gtx.Dp(ui.Space8) && q.x0 < p.x1+gtx.Dp(ui.Space8) {
				return false
			}
		}
		return true
	}
	order := make([]float64, 0, len(ticks))
	for _, t := range ticks {
		if t == 0 {
			order = append([]float64{t}, order...)
		} else {
			order = append(order, t)
		}
	}
	height := 0
	for _, t := range order {
		block := textBlock{style: ui.TextCaption, content: spec.FormatValue(t), ink: colors.SecondaryInk, maxWidth: max(1, plan.trackMax-plan.trackMin), maxLines: 1}
		size := block.measure(gtx, view)
		x := int(math.Round(plan.scale.at(t))) - size.X/2
		x = max(plan.trackMin, min(x, plan.trackMax-size.X))
		p := placed{x, x + size.X}
		if !fits(p) {
			continue
		}
		shown = append(shown, p)
		block.draw(gtx, view, image.Pt(x, y+gap))
		height = max(height, size.Y+gap)
	}
	return height
}
