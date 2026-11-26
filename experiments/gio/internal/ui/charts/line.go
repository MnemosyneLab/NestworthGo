package charts

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"time"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

// LinePoint is one observation of a series.
type LinePoint struct {
	// ID is stable across refreshes, for example the civil date.
	ID ID
	// Time positions the point. Civil dates are carried as UTC midnight and
	// the time axis is read in UTC, so a date never shifts with the viewer's
	// zone.
	Time time.Time
	// Value positions the point vertically. A missing value leaves a gap.
	Value Value
	// Text is the exact value as the caller formats it, for example the
	// currency amount. It appears in the tooltip and the readout.
	Text string
	// Detail adds lines to the tooltip and readout, such as the data source.
	Detail []string
	// Provisional draws the edges that touch the point dashed and the marker
	// hollow, as an estimate is distinguished from a recorded value.
	Provisional bool
	// BreakBefore starts a new line at this point even when the previous point
	// has a value, for a boundary the owning read model declares.
	BreakBefore bool
}

// LineSeries is one line. Points are drawn in the order given and are never
// sorted, so the caller supplies them in time order.
type LineSeries struct {
	ID   ID
	Name string
	// Color overrides the theme color. The zero color uses the theme's
	// category palette by series position.
	Color  color.NRGBA
	Points []LinePoint
}

// LineSpec describes a line chart: wealth history, return trends and any other
// value over time.
type LineSpec struct {
	// Label names the chart for assistive technology; Summary is the caller's
	// one-sentence description, shown while no item is active.
	Label, Summary string
	Series         []LineSeries
	// From and To fix the time viewport. The zero time derives it from the
	// points, missing ones included.
	From, To time.Time
	// IncludeZero extends the value axis to zero. The axis always includes zero
	// when the data crosses it.
	IncludeZero bool
	// Area fills under each line; use it for a single series.
	Area bool
	// FormatTime writes the date in the readout and tooltip, in full.
	FormatTime func(time.Time) string
	// FormatTick labels the time axis. Give it a compact form such as month and
	// year; it defaults to FormatTime.
	FormatTick func(time.Time) string
	// FormatValue labels value ticks. It is display rounding only.
	FormatValue func(float64) string
	Selected    Selection
	Messages    Messages
	// Height is the plot height including the axis labels. Zero uses 260dp.
	Height unit.Dp
}

// Rows returns the text form of every point, in series and point order.
func (s LineSpec) Rows() []TextRow {
	var rows []TextRow
	for _, series := range s.Series {
		for _, p := range series.Points {
			value, missing := valueText(p.Value, p.Text, s.Messages)
			detail := append([]string{s.formatTime(p.Time)}, p.Detail...)
			rows = append(rows, TextRow{
				Series: series.ID, ID: p.ID, Label: series.Name, Value: value, Detail: detail,
				Missing: missing, Selected: s.Selected == Selection{Series: series.ID, Item: p.ID},
			})
		}
	}
	return rows
}

// Validate lists empty and repeated series IDs, and empty and repeated point
// IDs within a series. Charts do not call it; pages and tests do, because a
// duplicate would make two items select together.
func (s LineSpec) Validate() []Problem {
	var series idCheck
	var problems []Problem
	for _, sr := range s.Series {
		series.add("", sr.ID)
		var points idCheck
		for _, p := range sr.Points {
			points.add(sr.ID, p.ID)
		}
		problems = append(problems, points.problems...)
	}
	return append(series.problems, problems...)
}

func (s LineSpec) formatTime(t time.Time) string {
	if s.FormatTime != nil {
		return s.FormatTime(t)
	}
	return t.UTC().Format("2006-01-02")
}

func (s LineSpec) formatTick(t time.Time) string {
	if s.FormatTick != nil {
		return s.FormatTick(t)
	}
	return s.formatTime(t)
}

func (s LineSpec) formatValue(v float64) string {
	if s.FormatValue != nil {
		return s.FormatValue(v)
	}
	return strconv.FormatFloat(v, 'g', 6, 64)
}

func (s LineSpec) pointCount() int {
	n := 0
	for _, series := range s.Series {
		n += len(series.Points)
	}
	return n
}

// yTickBudget is how many value ticks fit a plot of the given height when
// each needs spacing pixels. It is between two and six: a short chart keeps
// its two end labels rather than a taller minimum height.
func yTickBudget(plotHeight, spacing int) int {
	return min(6, max(2, plotHeight/max(1, spacing)))
}

// lineDomain is the data extent of a line chart.
type lineDomain struct {
	from, to time.Time
	// ticks are the value ticks; the plotted domain is first to last.
	yTicks []float64
	// plottable is false when no point has a value.
	plottable bool
}

func (s LineSpec) domain(maxYTicks int) lineDomain {
	d := lineDomain{from: s.From, to: s.To}
	lo, hi := math.Inf(1), math.Inf(-1)
	var from, to time.Time
	for _, series := range s.Series {
		for _, p := range series.Points {
			if from.IsZero() || p.Time.Before(from) {
				from = p.Time
			}
			if to.IsZero() || p.Time.After(to) {
				to = p.Time
			}
			if p.Value.Valid() {
				lo, hi = math.Min(lo, p.Value.Number), math.Max(hi, p.Value.Number)
			}
		}
	}
	if d.from.IsZero() {
		d.from = from
	}
	if d.to.IsZero() {
		d.to = to
	}
	if math.IsInf(lo, 1) {
		return d
	}
	d.plottable = true
	if s.IncludeZero {
		lo, hi = math.Min(lo, 0), math.Max(hi, 0)
	}
	d.yTicks = niceTicks(lo, hi, maxYTicks)
	return d
}

// linePointPlan is a point after layout.
type linePointPlan struct {
	sel         Selection
	at          f32.Point
	present     bool
	provisional bool
}

// lineRun is a stroked run of connected points with one line style.
type lineRun struct {
	points []f32.Point
	dashed bool
}

type lineSeriesPlan struct {
	id     ID
	ink    color.NRGBA
	marker markerShape
	points []linePointPlan
	runs   []lineRun
	// areas are the polylines to fill under, one per solid or dashed chain.
	chains [][]f32.Point
	// isolated indexes points with no neighbor, drawn as markers because they
	// have no line.
	isolated []int
	// last is the index of the final point that has a value, or -1.
	last int
}

// linePlan is a line chart's geometry for one plot rectangle.
type linePlan struct {
	plot       image.Rectangle
	x, y       scale
	yTicks     []float64
	xTicks     []time.Time
	zero       float64
	zeroInside bool
	series     []lineSeriesPlan
	order      []Selection
	plottable  bool
	// missingMark is the pixel height of the mark drawn for a missing point.
	missingMark int
}

func (s LineSpec) plan(d lineDomain, plot image.Rectangle, maxXTicks, missingMark int, colors *ui.Palette) linePlan {
	p := linePlan{plot: plot, plottable: d.plottable && plot.Dx() > 0 && plot.Dy() > 0, missingMark: missingMark}
	if !p.plottable {
		return p
	}
	p.yTicks = d.yTicks
	lo, hi := d.yTicks[0], d.yTicks[len(d.yTicks)-1]
	p.y = scale{d0: hi, d1: lo, r0: float64(plot.Min.Y), r1: float64(plot.Max.Y)}
	from, to := float64(d.from.UnixNano())/1e9, float64(d.to.UnixNano())/1e9
	p.x = scale{d0: from, d1: to, r0: float64(plot.Min.X), r1: float64(plot.Max.X)}
	p.xTicks = timeTicks(d.from, d.to, maxXTicks)
	p.zero = p.y.at(0)
	p.zeroInside = lo < 0 && hi > 0

	for i, series := range s.Series {
		sp := lineSeriesPlan{id: series.ID, ink: series.Color, marker: markerShape(i) % markerShapes, last: -1}
		if sp.ink == (color.NRGBA{}) {
			sp.ink = colors.Category[i%len(colors.Category)]
		}
		var chain []int
		flush := func() {
			sp.addChain(chain)
			chain = nil
		}
		for j, pt := range series.Points {
			sel := Selection{Series: series.ID, Item: pt.ID}
			pp := linePointPlan{sel: sel, present: pt.Value.Valid(), provisional: pt.Provisional}
			pp.at.X = float32(p.x.at(float64(pt.Time.UnixNano()) / 1e9))
			if pp.present {
				pp.at.Y = float32(p.y.at(pt.Value.Number))
			} else {
				pp.at.Y = float32(plot.Max.Y - missingMark/2)
			}
			sp.points = append(sp.points, pp)
			p.order = append(p.order, sel)
			if pt.BreakBefore || !pp.present {
				flush()
			}
			if pp.present {
				chain = append(chain, j)
				sp.last = j
			}
		}
		flush()
		p.series = append(p.series, sp)
	}
	return p
}

// addChain splits a chain of consecutive present points into runs. An edge is
// dashed when either end is provisional.
func (sp *lineSeriesPlan) addChain(chain []int) {
	switch len(chain) {
	case 0:
		return
	case 1:
		sp.isolated = append(sp.isolated, chain[0])
		return
	}
	var all []f32.Point
	var current *lineRun
	for k, idx := range chain {
		at := sp.points[idx].at
		all = append(all, at)
		if k == 0 {
			continue
		}
		prev := sp.points[chain[k-1]]
		dashed := prev.provisional || sp.points[idx].provisional
		if current == nil || current.dashed != dashed {
			sp.runs = append(sp.runs, lineRun{points: []f32.Point{prev.at}, dashed: dashed})
			current = &sp.runs[len(sp.runs)-1]
		}
		current.points = append(current.points, at)
	}
	sp.chains = append(sp.chains, all)
}

func (p linePlan) lookup(sel Selection) (seriesIndex, pointIndex int, ok bool) {
	for i, s := range p.series {
		if s.id != sel.Series {
			continue
		}
		for j, pt := range s.points {
			if pt.sel == sel {
				return i, j, true
			}
		}
	}
	return 0, 0, false
}

// anchor is the pixel position of an item, for tooltips and emphasis.
func (p linePlan) anchor(sel Selection) (image.Point, bool) {
	i, j, ok := p.lookup(sel)
	if !ok {
		return image.Point{}, false
	}
	at := p.series[i].points[j].at
	return image.Pt(int(math.Round(float64(at.X))), int(math.Round(float64(at.Y)))), true
}

// pick finds the item under a pointer. It chooses the nearest time column
// first, as a time-axis tooltip does, then the series closest in height within
// that column. A pointer farther than radius pixels from every column picks
// nothing.
func (p linePlan) pick(at image.Point, radius int) (Selection, bool) {
	if !p.plottable || at.Y < p.plot.Min.Y-radius || at.Y > p.plot.Max.Y+radius {
		return Selection{}, false
	}
	bestDX := math.Inf(1)
	for _, s := range p.series {
		for _, pt := range s.points {
			bestDX = math.Min(bestDX, math.Abs(float64(pt.at.X)-float64(at.X)))
		}
	}
	if bestDX > float64(radius) {
		return Selection{}, false
	}
	var best Selection
	bestDY := math.Inf(1)
	for _, s := range p.series {
		for _, pt := range s.points {
			if math.Abs(float64(pt.at.X)-float64(at.X)) > bestDX+0.5 {
				continue
			}
			if dy := math.Abs(float64(pt.at.Y) - float64(at.Y)); dy < bestDY {
				best, bestDY = pt.sel, dy
			}
		}
	}
	return best, !best.IsZero()
}

// move implements keyboard navigation. Left and Right step along the series of
// the cursor; Up and Down switch series at the nearest time; Home and End jump
// to the series ends.
func (p linePlan) move(from Selection, name key.Name) (Selection, bool) {
	if len(p.order) == 0 {
		return Selection{}, false
	}
	si, pi, ok := p.lookup(from)
	if !ok {
		return p.order[0], true
	}
	series := p.series[si]
	switch name {
	case key.NameLeftArrow:
		return series.points[max(0, pi-1)].sel, true
	case key.NameRightArrow:
		return series.points[min(len(series.points)-1, pi+1)].sel, true
	case key.NameHome:
		return series.points[0].sel, true
	case key.NameEnd:
		return series.points[len(series.points)-1].sel, true
	case key.NameUpArrow, key.NameDownArrow:
		step := 1
		if name == key.NameUpArrow {
			step = -1
		}
		for target := si + step; target >= 0 && target < len(p.series); target += step {
			candidate := p.series[target]
			if len(candidate.points) == 0 {
				continue
			}
			best, bestDX := 0, math.Inf(1)
			for j, pt := range candidate.points {
				if dx := math.Abs(float64(pt.at.X - series.points[pi].at.X)); dx < bestDX {
					best, bestDX = j, dx
				}
			}
			return candidate.points[best].sel, true
		}
		return from, true
	}
	return Selection{}, false
}

// LineChart is the widget state of one line chart. Keep one value per chart
// across frames; it holds the pointer hover, the keyboard cursor and nothing
// else. The selection belongs to the owner and arrives through LineSpec.
type LineChart struct {
	interaction
}

const (
	ringPad        = ui.Space4
	lineWidth      = 2.5
	defaultHeight  = unit.Dp(260)
	hitRadius      = unit.Dp(28)
	missingMarkDp  = unit.Dp(8)
	tickGap        = unit.Dp(8)
	minMarkerSpace = unit.Dp(14)
)

// Layout draws the chart at the width of gtx.Constraints.Max and returns the
// selection requests made this frame.
func (c *LineChart) Layout(gtx layout.Context, view ui.View, spec LineSpec) (layout.Dimensions, []Event) {
	width := gtx.Constraints.Max.X
	pad := gtx.Dp(ringPad)
	inner := max(1, width-2*pad)
	colors := view.Colors
	plotHeight := gtx.Dp(spec.Height)
	if plotHeight == 0 {
		plotHeight = gtx.Dp(defaultHeight)
	}

	if spec.pointCount() == 0 {
		return drawNotice(gtx, view, image.Pt(width, plotHeight), spec.Messages.Empty), nil
	}

	y := pad
	legend := c.layoutLegend(gtx, view, spec, image.Pt(pad, y), inner)
	if legend > 0 {
		y += legend + gtx.Dp(ui.Space8)
	}

	// Measure the labels that set the plot margins.
	labelStyle := ui.TextCaption
	label := func(s string) textBlock {
		return textBlock{style: labelStyle, content: s, ink: colors.SecondaryInk, maxWidth: inner, maxLines: 1}
	}
	lineHeight := label("0").measure(gtx, view).Y
	maxYTicks := yTickBudget(plotHeight, gtx.Dp(44))
	domain := spec.domain(maxYTicks)
	if !domain.plottable {
		dims := drawNotice(gtx, view, image.Pt(width, plotHeight), spec.Messages.Missing)
		return dims, nil
	}
	yLabelWidth := 0
	for _, tick := range domain.yTicks {
		yLabelWidth = max(yLabelWidth, label(spec.formatValue(tick)).measure(gtx, view).X)
	}
	tickWidth := func(t time.Time) int { return label(spec.formatTick(t)).measure(gtx, view).X }
	xLabelWidth := max(tickWidth(domain.from), tickWidth(domain.to))
	gap := gtx.Dp(tickGap)
	left := max(yLabelWidth+gap, xLabelWidth/2)
	right := max(xLabelWidth/2, gtx.Dp(ui.Space8))
	top := gtx.Dp(ui.Space8)
	bottom := lineHeight + gap
	plot := image.Rect(pad+left, y+top, pad+inner-right, y+plotHeight-bottom)
	if plot.Dx() < gtx.Dp(40) {
		plot.Max.X = plot.Min.X + gtx.Dp(40)
	}
	// Fit as many calendar ticks as the real labels allow: the end labels only
	// estimate the width, so measure the chosen ticks and retry if they are
	// wider.
	maxXTicks := 1
	for range 3 {
		maxXTicks = max(1, plot.Dx()/(xLabelWidth+gtx.Dp(ui.Space12)))
		widest := 0
		for _, t := range timeTicks(domain.from, domain.to, maxXTicks) {
			widest = max(widest, tickWidth(t))
		}
		if widest <= xLabelWidth {
			break
		}
		xLabelWidth = widest
	}
	plan := spec.plan(domain, plot, maxXTicks, gtx.Dp(missingMarkDp), colors)

	items := frameItems{order: plan.order, selected: spec.Selected, move: plan.move,
		pick: func(at image.Point) (Selection, bool) { return plan.pick(at, gtx.Dp(hitRadius)) }}
	events := c.update(gtx, c, items)
	active := c.active(gtx, c, plan.order)

	dims := accessibility.List(gtx, c, spec.Label, func(gtx layout.Context) layout.Dimensions {
		c.drawPlot(gtx, view, spec, plan, active, label)
		y += plotHeight

		shown := active
		if shown.IsZero() {
			shown = spec.Selected
		}
		y += gtx.Dp(ui.Space8)
		rgtx := gtx
		rgtx.Constraints.Max.X = inner
		offset := op.Offset(image.Pt(pad, y)).Push(gtx.Ops)
		read := layoutReadout(rgtx, view, spec.Messages, spec.Summary, spec.readout(shown))
		offset.Pop()
		y += read.Size.Y + pad

		size := image.Pt(width, y)
		c.semantics(gtx, spec, plan, active)
		register(gtx, c, size, !c.hover.IsZero())
		if gtx.Focused(c) {
			colors.FocusRing(gtx, image.Rectangle{Min: image.Pt(pad, pad), Max: image.Pt(pad+inner, y-pad)}, ui.RadiusMD)
		}
		return layout.Dimensions{Size: size}
	})
	return dims, events
}

// readout builds the text shown for sel, or nil when nothing is selected.
func (s LineSpec) readout(sel Selection) *readout {
	if sel.IsZero() {
		return nil
	}
	for _, series := range s.Series {
		if series.ID != sel.Series {
			continue
		}
		for _, p := range series.Points {
			if p.ID != sel.Item {
				continue
			}
			value, missing := valueText(p.Value, p.Text, s.Messages)
			if len(s.Series) > 1 {
				value = series.Name + ": " + value
			}
			return &readout{
				label: s.formatTime(p.Time), value: value, detail: p.Detail,
				selected: sel == s.Selected, missing: missing,
			}
		}
	}
	return nil
}

func (c *LineChart) layoutLegend(gtx layout.Context, view ui.View, spec LineSpec, at image.Point, width int) int {
	if len(spec.Series) < 2 {
		return 0
	}
	colors := view.Colors
	defer op.Offset(at).Push(gtx.Ops).Pop()
	gtx.Constraints.Max.X = width
	radius := gtx.Dp(5)
	dims := ui.Wrap{Gap: ui.Space12}.Layout(gtx, len(spec.Series), func(gtx layout.Context, i int) layout.Dimensions {
		series := spec.Series[i]
		ink := series.Color
		if ink == (color.NRGBA{}) {
			ink = colors.Category[i%len(colors.Category)]
		}
		block := textBlock{style: ui.TextCaption, content: series.Name, ink: colors.Ink, maxWidth: max(1, width-2*radius-gtx.Dp(ui.Space6))}
		size := block.measure(gtx, view)
		h := max(size.Y, 2*radius)
		drawMarker(gtx, markerShape(i)%markerShapes, f32.Pt(float32(radius), float32(h)/2), float32(radius)*0.85, ink)
		block.draw(gtx, view, image.Pt(2*radius+gtx.Dp(ui.Space6), (h-size.Y)/2))
		return layout.Dimensions{Size: image.Pt(2*radius+gtx.Dp(ui.Space6)+size.X, h)}
	})
	return dims.Size.Y
}

func (c *LineChart) drawPlot(gtx layout.Context, view ui.View, spec LineSpec, plan linePlan, active Selection, label func(string) textBlock) {
	colors := view.Colors
	plot := plan.plot
	gap := gtx.Dp(tickGap)

	// Value grid and labels. The zero line is stronger than the grid so a
	// negative stretch reads against a clear baseline.
	for _, tick := range plan.yTicks {
		py := int(math.Round(plan.y.at(tick)))
		ink, thickness := colors.Divider, 1
		if tick == 0 && plan.zeroInside {
			ink, thickness = ui.WithAlpha(colors.Ink, 0.4), max(1, gtx.Dp(1)+gtx.Dp(1)/2)
		}
		hairline(gtx, image.Pt(plot.Min.X, py), image.Pt(plot.Max.X, py), thickness, ink)
		block := label(spec.formatValue(tick))
		block.align = text.End
		block.maxWidth = plot.Min.X - gap - gtx.Dp(ringPad)
		size := block.measure(gtx, view)
		block.maxWidth = max(block.maxWidth, size.X)
		block.draw(gtx, view, image.Pt(plot.Min.X-gap-block.maxWidth, py-size.Y/2))
	}
	hairline(gtx, image.Pt(plot.Min.X, plot.Max.Y), image.Pt(plot.Max.X, plot.Max.Y), 1, ui.WithAlpha(colors.Ink, 0.25))
	for _, tick := range plan.xTicks {
		px := int(math.Round(plan.x.at(float64(tick.UnixNano()) / 1e9)))
		hairline(gtx, image.Pt(px, plot.Max.Y), image.Pt(px, plot.Max.Y+gtx.Dp(ui.Space4)), 1, ui.WithAlpha(colors.Ink, 0.25))
		block := label(spec.formatTick(tick))
		size := block.measure(gtx, view)
		block.draw(gtx, view, image.Pt(px-size.X/2, plot.Max.Y+gap))
	}

	markerRadius := float32(gtx.Dp(4))
	func() {
		defer clip.Rect(plot.Inset(-int(markerRadius) - gtx.Dp(2))).Push(gtx.Ops).Pop()
		c.drawSeries(gtx, view, spec, plan, active, markerRadius)
	}()
	if at, ok := plan.anchor(active); ok && (!c.hover.IsZero() || gtx.Focused(c)) {
		if item := spec.readout(active); item != nil {
			bounds := image.Rectangle{Min: image.Pt(0, 0), Max: image.Pt(plot.Max.X+gtx.Dp(ringPad)+gtx.Dp(ui.Space8), plot.Max.Y+gap)}
			drawTooltip(gtx, view, bounds, at, *item)
		}
	}
}

func (c *LineChart) drawSeries(gtx layout.Context, view ui.View, spec LineSpec, plan linePlan, active Selection, markerRadius float32) {
	colors := view.Colors
	plot := plan.plot
	focusSeries := active.Series
	if active.IsZero() {
		focusSeries = spec.Selected.Series
	}
	emphasis := len(plan.series) > 1 && focusSeries != ""
	spacing := math.Inf(1)
	for _, s := range plan.series {
		if len(s.points) > 1 {
			spacing = math.Min(spacing, float64(plot.Dx())/float64(len(s.points)-1))
		}
	}
	baseline := float32(min(max(plan.zero, float64(plot.Min.Y)), float64(plot.Max.Y)))
	// A light stroke color over a dark page reads stronger than the same
	// alpha over a light one, so the dark fill is thinner.
	areaAlpha := 0.12
	if colors.Dark {
		areaAlpha = 0.07
	}
	for _, s := range plan.series {
		ink := s.ink
		width := float32(gtx.Dp(lineWidth))
		if emphasis && s.id != focusSeries {
			ink = ui.WithAlpha(ink, 0.5)
		} else if emphasis {
			width *= 1.2
		}
		if spec.Area && len(plan.series) == 1 {
			for _, chain := range s.chains {
				poly := append([]f32.Point{{X: chain[0].X, Y: baseline}}, chain...)
				poly = append(poly, f32.Pt(chain[len(chain)-1].X, baseline))
				fillPolygon(gtx, poly, ui.WithAlpha(ink, areaAlpha))
			}
		}
		for _, run := range s.runs {
			if !run.dashed {
				strokePolyline(gtx, run.points, width, ink)
				continue
			}
			for _, dash := range dashPolyline(run.points, float32(gtx.Dp(6)), float32(gtx.Dp(4))) {
				strokePolyline(gtx, dash, width, ink)
			}
		}
	}
	// A missing point has no mark on the line, so it gets a short tick on the
	// time axis. A gap in the line then never reads as a zero.
	for _, s := range plan.series {
		for _, pt := range s.points {
			if !pt.present {
				x := int(math.Round(float64(pt.at.X)))
				hairline(gtx, image.Pt(x, plot.Max.Y-plan.missingMark), image.Pt(x, plot.Max.Y), max(1, gtx.Dp(1)+gtx.Dp(1)/2), ui.WithAlpha(colors.SecondaryInk, 0.8))
			}
		}
	}
	showAll := spacing >= float64(gtx.Dp(minMarkerSpace))
	for _, s := range plan.series {
		mark := func(i int) {
			pt := s.points[i]
			if !pt.present {
				return
			}
			drawMarker(gtx, s.marker, pt.at, markerRadius, s.ink)
			if pt.provisional {
				// A hollow marker flags an estimate without relying on color.
				drawMarker(gtx, s.marker, pt.at, markerRadius*0.5, colors.Surface)
			}
		}
		if showAll {
			for i := range s.points {
				mark(i)
			}
		}
		for _, i := range s.isolated {
			mark(i)
		}
		if s.last >= 0 {
			mark(s.last)
		}
	}
	// Active and selected points sit above the series. The active point draws a
	// guide to the axis; the selected point carries an ink ring and a firmer
	// guide, so the two states differ in form as well as in color.
	if si, pi, ok := plan.lookup(active); ok {
		pt := plan.series[si].points[pi]
		x := int(math.Round(float64(pt.at.X)))
		hairline(gtx, image.Pt(x, plot.Min.Y), image.Pt(x, plot.Max.Y), 1, ui.WithAlpha(colors.Ink, 0.35))
		if pt.present {
			drawRingedMarker(gtx, plan.series[si].marker, pt.at, markerRadius*1.3, float32(gtx.Dp(2)), plan.series[si].ink, colors.Surface)
		}
	}
	if si, pi, ok := plan.lookup(spec.Selected); ok {
		pt := plan.series[si].points[pi]
		x := int(math.Round(float64(pt.at.X)))
		hairline(gtx, image.Pt(x, plot.Min.Y), image.Pt(x, plot.Max.Y), max(1, gtx.Dp(1)), ui.WithAlpha(colors.Ink, 0.7))
		if pt.present {
			drawRingedMarker(gtx, plan.series[si].marker, pt.at, markerRadius*1.3, float32(gtx.Dp(2)), plan.series[si].ink, colors.Ink)
		}
	}
}

// semantics adds a text node per point for assistive technology. A long
// series would add thousands of nodes, so beyond semanticLimit only the
// summary, the active point and the selected point are described.
func (c *LineChart) semantics(gtx layout.Context, spec LineSpec, plan linePlan, active Selection) {
	semanticItem(gtx, plan.plot, spec.Summary)
	describe := func(sel Selection) {
		item := spec.readout(sel)
		at, ok := plan.anchor(sel)
		if item == nil || !ok {
			return
		}
		r := gtx.Dp(ui.Space8)
		semanticItem(gtx, image.Rect(at.X-r, at.Y-r, at.X+r, at.Y+r), itemLabel(*item, spec.Messages))
	}
	if len(plan.order) <= semanticLimit {
		for _, sel := range plan.order {
			describe(sel)
		}
		return
	}
	describe(active)
	if spec.Selected != active {
		describe(spec.Selected)
	}
}

// semanticLimit bounds the per-point accessibility nodes of one chart.
const semanticLimit = 120

func itemLabel(r readout, m Messages) string {
	label := r.label
	if r.value != "" {
		label += ", " + r.value
	}
	for _, d := range r.detail {
		label += ", " + d
	}
	if r.selected && m.Selected != "" {
		label = m.Selected + ", " + label
	}
	return label
}
