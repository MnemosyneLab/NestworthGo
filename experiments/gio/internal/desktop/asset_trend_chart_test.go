package desktop

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestAssetTrendLineKeepsEmptyPeriodsAndKnownSubtotals(t *testing.T) {
	view := ui.View{Language: i18n.English, DateFormat: settings.DateFormatISO, DecimalSeparator: ".", GroupingSeparator: ",", Colors: &lightPalette}
	q := domain.AnalysisQuery{From: "2026-08-04", To: "2026-08-26"}
	quoteDay := "2026-08-23"
	legacy, err := domain.NewSignedMoney(decimal.NewFromInt(90), "USD")
	if err != nil {
		t.Fatal(err)
	}
	data := application.AssetTrendResult{Points: []application.AssetTrendPoint{
		{Period: "2026-08-03", Value: &legacy},
		{Period: "2026-08-10", Value: &legacy, DisplayValuation: &domain.ValuationAmount{
			Currency: "USD", KnownSubtotal: &domain.MoneyView{Amount: "45.25", Currency: "USD"},
			Quality: domain.ValuationQuality{Availability: domain.ValuationPartial, Coverage: domain.CoverageUnavailable, UnpricedCount: 1}}},
		{Period: "2026-08-24", DisplayValuation: &domain.ValuationAmount{
			Currency: "USD", Total: &domain.MoneyView{Amount: "102.75", Currency: "USD"},
			Quality: domain.ValuationQuality{Availability: domain.ValuationFull, Coverage: domain.CoverageProvisional, OldestMarketDate: &quoteDay}}},
	}}
	spec, missing := assetTrendLine(view, data, q, application.TrendNetWorth, application.TrendWeek)
	points := spec.Series[0].Points
	if len(points) != 4 || missing != 2 || len(spec.Validate()) != 0 || spec.From.Format(time.DateOnly) != q.From || spec.To.Format(time.DateOnly) != q.To {
		t.Fatalf("weekly coverage or viewport changed: %+v", spec)
	}
	if points[0].ID != "2026-08-03" || points[0].Time.Format(time.DateOnly) != q.From || points[0].Value != charts.Num(90) ||
		points[1].Value.Valid() || points[2].ID != "2026-08-17" || points[2].Value.Valid() || !points[3].Provisional || points[3].Value != charts.Num(102.75) {
		t.Fatalf("weekly observations lost the clamped first week, a gap or provisional value: %+v", points)
	}
	table := spec.Rows()
	if !strings.Contains(analysisLineTableText(table[1]), "Known net contribution · US$45.25") ||
		!strings.Contains(analysisLineTableText(table[3]), "Quote dates 2026-08-23") {
		t.Fatalf("the readable data lost evidence: %+v", table)
	}
	months := assetTrendPeriods("2026-02-24", "2026-04-04", application.TrendMonth)
	if len(months) != 3 || months[0].key != "2026-02" || months[0].day.Format(time.DateOnly) != "2026-02-24" || months[2].key != "2026-04" {
		t.Fatalf("month boundaries changed: %+v", months)
	}
}

func TestAssetTrendUsesSignedDebtLevelsAndExactRates(t *testing.T) {
	view := ui.View{Language: i18n.English, DecimalSeparator: ".", GroupingSeparator: ","}
	level := domain.ValuationAmount{Currency: "USD", Total: &domain.MoneyView{Amount: "25.10", Currency: "USD"},
		Quality: domain.ValuationQuality{Availability: domain.ValuationFull}}
	rate := decimal.RequireFromString("0.0525")
	data := application.AssetTrendPoint{DisplayValuation: &level, Rate: &rate, Coverage: domain.RateCoverage{RatedDays: 1, TotalDays: 1}}
	period := assetTrendPeriod{key: "2026-08-04"}
	debt := assetTrendChartPoint(view, application.TrendLiabilities, period, data)
	if debt.Value != charts.Num(-25.1) || debt.Text != "-US$25.10" {
		t.Fatalf("debt level sign changed: %+v", debt)
	}
	point := assetTrendChartPoint(view, application.TrendReturnRate, period, data)
	if point.Value != charts.Num(0.0525) || point.Text != "+5.25%" || len(point.Detail) != 1 || point.Detail[0] != "Rated days 1/1" {
		t.Fatalf("return rate or coverage changed: %+v", point)
	}
	data.Rate = nil
	if gap := assetTrendChartPoint(view, application.TrendReturnRate, period, data); gap.Value.Valid() {
		t.Fatal("the return rate used a valuation as an available percentage")
	}
}
