package charts

import (
	"image"
	"strings"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func compactLegendSpec(label, amount, share string) DonutSpec {
	return DonutSpec{Label: "Composition", Compact: true, NotDrawn: "Not in ring",
		Messages: Messages{Missing: "No value", Selected: "Selected", Empty: "No data"},
		Slices: []Slice{
			{ID: "cash", Label: "Cash", Value: Num(60), Text: "US$60.00", Share: "60.0%"},
			{ID: "long", Label: label, Value: Num(-40), Text: amount, Share: share},
		}}
}

func compactLegendFor(view ui.View, spec DonutSpec, legendWidth int) (layout.Context, compactLegend, []donutRow) {
	gtx := layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(legendWidth, 4000)}}
	rows := make([]donutRow, len(spec.Slices))
	for i, sl := range spec.Slices {
		rows[i].amount, _ = valueText(sl.Value, sl.Text, spec.Messages)
		if !(sl.Value.Valid() && sl.Value.Number > 0) {
			rows[i].note = spec.NotDrawn
		}
	}
	return gtx, newCompactLegend(gtx, view, spec, rows, legendWidth, gtx.Dp(ui.Space4), gtx.Dp(10)), rows
}

// The compact legend measures, places and draws from one width budget: the
// figures never cover the category, and an amount that does not fit beside it
// moves under it and wraps instead of being cut.
func TestCompactDonutLegendSharesOneWidthBudget(t *testing.T) {
	view := newRig(400, false).view
	textWidth := func(c compactLegend, legendWidth int) int { return legendWidth - c.labelX - c.inset }
	figuresWidth := func(c compactLegend) int {
		if c.chipBelow {
			return max(c.amountW, c.chipW)
		}
		return c.amountW + c.chipGap + c.chipW
	}
	lineHeight := func(gtx layout.Context, c compactLegend) int {
		return c.amountBlock("0", view.Colors.Ink).measure(gtx, view).Y
	}

	t.Run("short figures sit beside the category", func(t *testing.T) {
		gtx, c, _ := compactLegendFor(view, compactLegendSpec("Stocks", "-US$40.00", "40.0%"), 360)
		textW := textWidth(c, 360)
		if c.stacked || figuresWidth(c) > textW*compactFiguresShare/100 || c.labelW < gtx.Dp(compactLabelMin) {
			t.Fatalf("legend = %+v for text width %d, want figures beside a readable category", c, textW)
		}
		if c.labelW+gtx.Dp(ui.Space8)+figuresWidth(c) > textW {
			t.Fatalf("category %d and figures %d overlap within %d", c.labelW, figuresWidth(c), textW)
		}
	})

	t.Run("figures over the share limit stack", func(t *testing.T) {
		amount := "-US$12,345,678,901.23"
		gtx, c, rows := compactLegendFor(view, compactLegendSpec("Stocks", amount, "-123.45%"), 300)
		textW := textWidth(c, 300)
		natural := textBlock{style: ui.TextLabel, content: amount, maxWidth: 1 << 16, maxLines: 1}.natural(gtx, view)
		if natural <= textW*compactFiguresShare/100-c.chipGap-c.chipW {
			t.Fatalf("fixture amount %dpx fits beside the category in %dpx; widen it", natural, textW)
		}
		if !c.stacked || c.labelW != textW || figuresWidth(c) > textW {
			t.Fatalf("legend = %+v, want figures stacked under a full-width category", c)
		}
		block := c.amountBlock(rows[1].amount, view.Colors.Ink)
		if block.maxLines != 0 {
			t.Fatalf("a stacked amount must be allowed to wrap: %+v", block)
		}
		if natural > c.amountW && block.measure(gtx, view).Y <= lineHeight(gtx, c) {
			t.Fatalf("an amount of %dpx in %dpx neither fits nor wraps", natural, c.amountW)
		}
	})

	t.Run("a narrow card with a long category keeps every line", func(t *testing.T) {
		spec := compactLegendSpec(strings.Repeat("Long-term retirement savings ", 3), "-US$9,876,543,210.98", "-98.76%")
		gtx, c, rows := compactLegendFor(view, spec, 220)
		if !c.stacked || figuresWidth(c) > textWidth(c, 220) {
			t.Fatalf("legend = %+v, want a stacked legend inside 220px", c)
		}
		sl := spec.Slices[1]
		textH := c.textHeight(gtx, view, sl, rows[1])
		amountH := c.amountBlock(rows[1].amount, view.Colors.Ink).measure(gtx, view).Y
		if textH <= lineHeight(gtx, c) {
			t.Fatalf("the long category should wrap at %dpx, got height %d", c.labelW, textH)
		}
		if !c.chipBelow || amountH != lineHeight(gtx, c) {
			t.Fatalf("legend = %+v, amount height %d: the share should move under an amount that then fits one line", c, amountH)
		}
		if got := c.rowHeight(gtx, view, sl, rows[1]); got < textH+amountH {
			t.Fatalf("row height %d hides text: category and note %d, amount %d", got, textH, amountH)
		}
	})

	t.Run("the chart lays out at every width", func(t *testing.T) {
		spec := compactLegendSpec(strings.Repeat("Long category ", 4), "-US$9,876,543,210.98", "-98.76%")
		for _, width := range []int{560, 360, 300, 220, 160} {
			r := newRig(width, false)
			var chart DonutChart
			size, _ := r.frame(func(gtx layout.Context, v ui.View, _ Selection) (layout.Dimensions, []Event) {
				return chart.Layout(gtx, v, spec)
			}, Selection{})
			if size.X != width || size.Y <= 0 {
				t.Fatalf("width %d: size %v", width, size)
			}
		}
	})
}
