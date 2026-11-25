package desktop

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestContributionChartKeepsRankedSignsAndExactText(t *testing.T) {
	money := func(amount string) *domain.SignedMoney {
		t.Helper()
		v, err := domain.NewSignedMoney(decimal.RequireFromString(amount), "USD")
		if err != nil {
			t.Fatal(err)
		}
		return &v
	}
	view := ui.View{Language: i18n.English, DecimalSeparator: ".", GroupingSeparator: ","}
	rate := decimal.RequireFromString("0.0525")
	partial := application.AnalysisAvailability{Status: domain.CompletenessPartial}
	data := application.ContributionResult{GroupBy: application.ContributionGroupInstrument, ReturnType: application.ContributionTotalReturn,
		Rows: []application.ContributionRow{
			{Key: "loss", Label: "Loss", Amount: money("-2.50"), AnalysisAvailability: partial},
			{Key: "zero", Label: "Zero", Amount: money("0")},
			{Key: "missing", Label: "Missing"},
			{Key: "gain", Label: "Gain", Amount: money("5.25"), Rate: &rate},
		}}
	var p returnAnalysisPage
	spec := p.contributionBars(view, data, rankedContributionRows(data.Rows, contributionLargest, 10))
	rows := spec.Rows()
	if !spec.Symmetric || len(spec.Validate()) != 0 || len(rows) != 3 {
		t.Fatalf("ranking chart has an unexpected scale or set of bars: %+v", spec)
	}
	for index, want := range []struct {
		id     charts.ID
		amount string
		value  float64
	}{
		{"gain", "$ 5.25", 5.25}, {"loss", "-$ 2.50", -2.5}, {"zero", "$ 0.00", 0},
	} {
		if got := rows[index]; got.ID != want.id || got.Value != want.amount || got.Missing || spec.Bars[index].Value != charts.Num(want.value) {
			t.Fatalf("bar %d lost its ranked amount: %+v", index, got)
		}
	}
	if rows[0].Detail[0] != "Return rate · +5.25%" || rows[1].Detail[1] != "Incomplete valuation" {
		t.Fatalf("chart readout lost rate or status: %+v", rows)
	}
}
