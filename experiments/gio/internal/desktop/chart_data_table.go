package desktop

import (
	"image"
	"strings"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
)

const (
	// chartTableMaxHeight bounds an open table so a long history scrolls
	// inside it instead of pushing the rest of the page away.
	chartTableMaxHeight unit.Dp = 360
	// chartTableSample is how many rows are measured to size the columns, so
	// sizing stays cheap for a long history.
	chartTableSample         = 120
	chartTablePadY   unit.Dp = 6
	// chartTablePadEnd keeps the trailing numbers clear of the list's
	// floating scrollbar.
	chartTablePadStart unit.Dp = ui.Space8
	chartTablePadEnd   unit.Dp = 18
)

// chartDataTable is the "View data table" disclosure under a chart, as every
// Wails trend and composition chart offers it. It starts closed; when open it
// shows the chart's values with a header row, aligned numeric columns and the
// selected item's row highlighted.
type chartDataTable struct {
	toggle widget.Clickable
	open   bool
	list   widget.List
}

// layout draws the disclosure and, when open, the table. The table is built
// only while open, so a closed disclosure costs nothing for a long history.
// Without items it draws nothing.
func (t *chartDataTable) layout(view ui.View, items int, build func() charts.Table) layout.Widget {
	return t.layoutWithActions(view, items, build, nil)
}

// layoutWithActions is layout with an optional action under a row, such as a
// link to the finding behind a missing value. action returns nil for rows
// without one.
func (t *chartDataTable) layoutWithActions(view ui.View, items int, build func() charts.Table, action func(row int) layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if items == 0 {
			return layout.Dimensions{}
		}
		if t.toggle.Clicked(gtx) {
			t.open = !t.open
			gtx.Execute(op.InvalidateCmd{})
		}
		label := view.Language.Text(i18n.ChartDataTable)
		toggle := analysisDisclosure(view, &t.toggle, label, t.open)
		if !t.open {
			return toggle(gtx)
		}
		table := build()
		if len(table.Rows) == 0 || len(table.Columns) == 0 {
			return toggle(gtx)
		}
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
			layout.Rigid(toggle),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return t.table(gtx, view, label, table, action)
			}),
		)
	}
}

func (t *chartDataTable) table(gtx layout.Context, view ui.View, label string, table charts.Table, action func(int) layout.Widget) layout.Dimensions {
	colors := view.Colors
	widths := chartTableWidths(gtx, view, table, gtx.Constraints.Max.X-gtx.Dp(chartTablePadStart+chartTablePadEnd))
	gap := gtx.Dp(ui.Space16)
	cells := func(gtx layout.Context, texts []string, header bool) layout.Dimensions {
		children := make([]layout.FlexChild, len(table.Columns))
		for c, column := range table.Columns {
			children[c] = layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X, gtx.Constraints.Max.X = widths[c], widths[c]
				style, ink := ui.TextBody, colors.Ink
				if header {
					style, ink = ui.TextCaptionStrong, colors.SecondaryInk
				}
				content := ""
				if c < len(texts) {
					content = texts[c]
				}
				if column.Numeric {
					return view.LineEnd(gtx, style, content, ink)
				}
				dims := view.Body(style, content, ink)(gtx)
				dims.Size.X = widths[c]
				return dims
			})
		}
		return layout.Inset{Top: chartTablePadY, Bottom: chartTablePadY, Left: chartTablePadStart, Right: chartTablePadEnd}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Gap: gap}.Layout(gtx, children...)
		})
	}
	header := make([]string, len(table.Columns))
	for c, column := range table.Columns {
		header[c] = column.Title
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return cells(gtx, header, true) }),
		layout.Rigid(view.Hairline),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, gtx.Dp(chartTableMaxHeight))
			defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
			t.list.Axis = layout.Vertical
			return keyboardList(gtx, view, &t.list, label, len(table.Rows), func(gtx layout.Context, i int) layout.Dimensions {
				macro := op.Record(gtx.Ops)
				dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if i == 0 {
							return layout.Dimensions{}
						}
						return view.Hairline(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						// The row reads as one sentence, as the chart readout does.
						macro := op.Record(gtx.Ops)
						dims := cells(gtx, table.Rows[i], false)
						call := macro.Stop()
						defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
						semantic.LabelOp(chartTableRowText(table.Rows[i])).Add(gtx.Ops)
						call.Add(gtx.Ops)
						return dims
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if action == nil {
							return layout.Dimensions{}
						}
						if w := action(i); w != nil {
							return layout.Inset{Left: chartTablePadStart, Right: chartTablePadEnd, Bottom: chartTablePadY}.Layout(gtx, w)
						}
						return layout.Dimensions{}
					}),
				)
				call := macro.Stop()
				if i == table.Selected {
					bounds := image.Rectangle{Max: image.Pt(gtx.Constraints.Max.X, dims.Size.Y)}
					paint.FillShape(gtx.Ops, colors.Tint(colors.Accent, 0.08), clip.UniformRRect(bounds, gtx.Dp(ui.RadiusSM)).Op(gtx.Ops))
				}
				call.Add(gtx.Ops)
				return dims
			})
		}),
	)
}

// chartTableWidths gives every column its natural width. Spare room goes to
// the first column, so numbers gather on the trailing side. When the content
// is wider than the table, wrapping columns give up width first and every
// column shrinks only after that.
func chartTableRowText(cells []string) string {
	parts := make([]string, 0, len(cells))
	for _, cell := range cells {
		if cell != "" {
			parts = append(parts, cell)
		}
	}
	return strings.Join(parts, " · ")
}

// lineTableItems counts the points a line chart's table would list.
func lineTableItems(spec charts.LineSpec) int {
	count := 0
	for _, series := range spec.Series {
		count += len(series.Points)
	}
	return count
}

// chartTableLabels are the shared column titles; details is the title of the
// details column of a single-series line table.
func chartTableLabels(l i18n.Language, details i18n.MessageID) charts.TableLabels {
	return charts.TableLabels{Date: l.Text(i18n.ChartDate), Category: l.Text(i18n.ChartCategory),
		Amount: l.Text(i18n.ChartAmount), Share: l.Text(i18n.ChartShare), Details: l.Text(details)}
}

func chartTableWidths(gtx layout.Context, view ui.View, table charts.Table, avail int) []int {
	n := len(table.Columns)
	gap := gtx.Dp(ui.Space16)
	avail = max(n, avail-gap*(n-1))
	natural := make([]int, n)
	measure := func(c int, content string, style ui.TextStyle) {
		if content == "" {
			return
		}
		probe := gtx
		probe.Constraints = layout.Constraints{Max: image.Pt(1<<20, 1<<20)}
		macro := op.Record(gtx.Ops)
		natural[c] = max(natural[c], view.Line(probe, style, content, view.Colors.Ink).Size.X)
		macro.Stop()
	}
	for c, column := range table.Columns {
		measure(c, column.Title, ui.TextCaptionStrong)
	}
	step := max(1, len(table.Rows)/chartTableSample)
	for i := 0; i < len(table.Rows); i += step {
		for c, cell := range table.Rows[i] {
			if c < n {
				measure(c, cell, ui.TextBody)
			}
		}
	}
	if last := len(table.Rows) - 1; last >= 0 && last%step != 0 {
		for c, cell := range table.Rows[last] {
			if c < n {
				measure(c, cell, ui.TextBody)
			}
		}
	}
	widths := append([]int(nil), natural...)
	total := 0
	for _, w := range widths {
		total += w
	}
	if total <= avail {
		widths[0] += avail - total
		return widths
	}
	fixed, flexible := 0, 0
	for c, column := range table.Columns {
		if column.Wrap {
			flexible += natural[c]
		} else {
			fixed += natural[c]
		}
	}
	floor := gtx.Dp(96)
	if flexible > 0 && avail-fixed >= floor {
		room := avail - fixed
		for c, column := range table.Columns {
			if column.Wrap {
				widths[c] = max(1, natural[c]*room/flexible)
			}
		}
		return widths
	}
	for c := range widths {
		widths[c] = max(1, natural[c]*avail/total)
	}
	return widths
}
