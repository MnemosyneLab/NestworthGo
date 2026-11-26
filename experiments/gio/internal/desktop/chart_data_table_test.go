package desktop

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
)

func chartTableContext(width int) layout.Context {
	return layout.Context{Ops: new(op.Ops), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(width, 800))}
}

func TestChartDataTableSizesColumnsToContent(t *testing.T) {
	view := ui.View{Theme: ui.NewTheme(), Language: i18n.English, Colors: &lightPalette}
	table := charts.Table{Columns: []charts.TableColumn{{Title: "Date"}, {Title: "Net worth", Numeric: true},
		{Title: "Coverage", Wrap: true}}, Selected: charts.NoRow,
		Rows: [][]string{
			{"2026-11-23", "CN¥2,674,695.09", "Quote dates 2026-10-12 – 2026-10-27 · Carried or stale quotes 1/0"},
			{"2026-11-24", "Unavailable", "1 missing input"},
		}}
	wide := chartTableWidths(chartTableContext(900), view, table, 900)
	if wide[0] < 60 || wide[1] < 80 || wide[1] > 300 || wide[2] < 200 {
		t.Fatalf("columns did not fit their content in a wide table: %v", wide)
	}
	narrow := chartTableWidths(chartTableContext(420), view, table, 420)
	gap := 16 * (len(narrow) - 1)
	if narrow[0] < 60 || narrow[1] < 80 || narrow[0]+narrow[1]+narrow[2]+gap > 420 {
		t.Fatalf("a narrow table squeezed dates or amounts instead of wrapping coverage: %v", narrow)
	}
}
