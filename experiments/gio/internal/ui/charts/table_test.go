package charts

import (
	"slices"
	"testing"
	"time"
)

func TestLineTableJoinsSeriesByDate(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 8, d, 0, 0, 0, 0, time.UTC) }
	spec := LineSpec{
		Messages: Messages{Missing: "Unavailable"},
		Series: []LineSeries{
			{ID: "assets", Name: "Assets", Points: []LinePoint{
				{ID: "2026-08-01", Time: day(1), Value: Num(10), Text: "US$10.00"},
				{ID: "2026-08-03", Time: day(3), Value: Num(12), Text: "US$12.00"},
			}},
			{ID: "debts", Name: "Liabilities", Points: []LinePoint{
				{ID: "2026-08-02", Time: day(2), Value: Num(-4), Text: "-US$4.00"},
				{ID: "2026-08-03", Time: day(3), Detail: []string{"No quote"}},
			}},
		},
		Selected: Selection{Series: "debts", Item: "2026-08-03"},
	}
	table := spec.Table(TableLabels{Date: "Date", Details: "Coverage"})
	titles := []string{table.Columns[0].Title, table.Columns[1].Title, table.Columns[2].Title}
	if len(table.Columns) != 3 || !slices.Equal(titles, []string{"Date", "Assets", "Liabilities"}) || !table.Columns[1].Numeric {
		t.Fatalf("columns = %+v; a multi-series table has no details column", table.Columns)
	}
	want := [][]string{
		{"2026-08-01", "US$10.00", ""},
		{"2026-08-02", "", "-US$4.00"},
		{"2026-08-03", "US$12.00", "Unavailable"},
	}
	if len(table.Rows) != len(want) || table.Selected != 2 {
		t.Fatalf("rows = %q, selected %d", table.Rows, table.Selected)
	}
	for i := range want {
		if !slices.Equal(table.Rows[i], want[i]) {
			t.Fatalf("row %d = %q, want %q", i, table.Rows[i], want[i])
		}
	}

	single := LineSpec{Series: spec.Series[1:], Messages: spec.Messages}
	details := single.Table(TableLabels{Date: "Date", Details: "Coverage"})
	if len(details.Columns) != 3 || details.Columns[2].Title != "Coverage" || details.Rows[1][2] != "No quote" || details.Selected != NoRow {
		t.Fatalf("single-series table lost point details: %+v", details)
	}
}

func TestDonutTableKeepsExactTextAndShares(t *testing.T) {
	spec := DonutSpec{
		Messages: Messages{Missing: "Unavailable"},
		Slices: []Slice{
			{ID: "cash", Label: "Cash", Value: Num(100.5), Text: "US$100.50", Share: "60.1%"},
			{ID: "bonds", Label: "Bonds", Share: "0%"},
		},
		Selected: Selection{Item: "bonds"},
	}
	table := spec.Table(TableLabels{Category: "Category", Amount: "Amount", Share: "Share"})
	if len(table.Columns) != 3 || table.Selected != 1 ||
		!slices.Equal(table.Rows[0], []string{"Cash", "US$100.50", "60.1%"}) ||
		!slices.Equal(table.Rows[1], []string{"Bonds", "Unavailable", "0%"}) {
		t.Fatalf("donut table = %+v", table)
	}
	spec.Slices[0].Share, spec.Slices[1].Share = "", ""
	if columns := spec.Table(TableLabels{}).Columns; len(columns) != 2 {
		t.Fatalf("a donut without shares still has a share column: %+v", columns)
	}
}
