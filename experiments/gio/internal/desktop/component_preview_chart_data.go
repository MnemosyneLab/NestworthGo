package desktop

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/chartpreview"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// Everything below is synthetic. Amounts are exact decimals that the preview
// formats with the household's separators, the way a page formats a read-model
// amount. The charts only receive float positions and the finished text.

const previewChartCurrency domain.CurrencyCode = "USD"

func previewDecimal(value string) decimal.Decimal { return decimal.RequireFromString(value) }

func previewPlot(value decimal.Decimal) charts.Value { return charts.Num(value.InexactFloat64()) }

func previewMoney(view ui.View, value decimal.Decimal) string {
	return valuationMoney(view, domain.MoneyView{Amount: value.String(), Currency: previewChartCurrency}, false)
}

// previewSigned writes an explicit plus sign on gains.
func previewSigned(view ui.View, value decimal.Decimal) string {
	text := previewMoney(view, value)
	if value.IsPositive() {
		return "+" + text
	}
	return text
}

func previewShare(view ui.View, part, total decimal.Decimal) string {
	share := part.Div(total).Shift(2).StringFixed(1)
	return strings.Replace(share, ".", view.DecimalSeparator, 1) + "%"
}

func previewMessages(view ui.View) charts.Messages {
	l := view.Language
	return charts.Messages{Empty: l.Text(i18n.OverviewNoValue), Missing: l.Text(i18n.OverviewNoValue), Selected: l.Text(i18n.ChartSelected)}
}

func previewMonth(day time.Time) string { return day.UTC().Format("2006/01") }

// previewWealthLine is three weekly-looking monthly series with a gap in the
// liabilities, which interrupts both liabilities and the net worth they feed,
// and an estimated latest point.
func previewWealthLine(view ui.View, selected charts.Selection) charts.LineSpec {
	l := view.Language
	const months = 18
	start := time.Date(2025, time.June, 1, 0, 0, 0, 0, time.UTC)
	net := charts.LineSeries{ID: "net-worth", Name: l.Text(i18n.NetWorth)}
	assets := charts.LineSeries{ID: "assets", Name: l.Text(i18n.Assets)}
	liabilities := charts.LineSeries{ID: "liabilities", Name: l.Text(i18n.Liabilities)}
	for i := range months {
		day := start.AddDate(0, i, 0)
		id := charts.ID(day.Format(time.DateOnly))
		estimated := i == months-1
		var detail []string
		if estimated {
			detail = []string{l.Text(i18n.ChartSampleEstimated)}
		}
		a := decimal.NewFromFloat(1150000 + 14000*float64(i) + 18000*math.Sin(float64(i)/2)).Round(2)
		li := decimal.NewFromFloat(410000 - 5200*float64(i)).Round(2)
		assets.Points = append(assets.Points, charts.LinePoint{ID: id, Time: day, Value: previewPlot(a), Text: previewMoney(view, a),
			Detail: detail, Provisional: estimated})
		if i >= 7 && i <= 9 {
			gap := charts.LinePoint{ID: id, Time: day, Text: l.Text(i18n.OverviewNoValue)}
			liabilities.Points = append(liabilities.Points, gap)
			net.Points = append(net.Points, gap)
			continue
		}
		n := a.Sub(li)
		liabilities.Points = append(liabilities.Points, charts.LinePoint{ID: id, Time: day, Value: previewPlot(li), Text: previewMoney(view, li)})
		net.Points = append(net.Points, charts.LinePoint{ID: id, Time: day, Value: previewPlot(n), Text: previewMoney(view, n),
			Detail: detail, Provisional: estimated})
	}
	return charts.LineSpec{
		Label: l.Text(i18n.NetWorth), Summary: l.Text(i18n.ChartPreviewHint),
		Series:      []charts.LineSeries{net, assets, liabilities},
		FormatTime:  func(day time.Time) string { return ui.FormatDay(day, l, view.DateFormat) },
		FormatTick:  previewMonth,
		FormatValue: func(tick float64) string { return chartAxisNumber(view, tick) },
		Messages:    previewMessages(view), Selected: selected,
	}
}

// previewReturnLine is a monthly rate with a true zero, negative values, two
// stretches without a valuation, a declared break and an estimated end.
func previewReturnLine(view ui.View, selected charts.Selection) charts.LineSpec {
	l := view.Language
	rates := []string{"0.012", "-0.008", "0", "0.024", "", "", "-0.031", "", "0.006", "0.019", "0.027", "0.011"}
	series := charts.LineSeries{ID: "rate", Name: l.Text(i18n.ReturnRate)}
	for i, rate := range rates {
		day := time.Date(2025, time.September+time.Month(i), 1, 0, 0, 0, 0, time.UTC)
		point := charts.LinePoint{ID: charts.ID(day.Format(time.DateOnly)), Time: day}
		if rate == "" {
			point.Text = l.Text(i18n.OverviewNoValue)
		} else {
			value := previewDecimal(rate)
			point.Value, point.Text = previewPlot(value), returnRate(view, &value)
		}
		switch i {
		case 9:
			point.BreakBefore = true
		case len(rates) - 1:
			point.Provisional = true
			point.Detail = []string{l.Text(i18n.ChartSampleEstimated)}
		}
		series.Points = append(series.Points, point)
	}
	return charts.LineSpec{
		Label: l.Text(i18n.ReturnRate), Summary: l.Text(i18n.ChartPreviewHint),
		Series: []charts.LineSeries{series}, IncludeZero: true, Area: true,
		FormatTime: func(day time.Time) string { return ui.FormatDay(day, l, view.DateFormat) },
		FormatTick: previewMonth,
		FormatValue: func(tick float64) string {
			value := decimal.NewFromFloat(tick).Round(4)
			return returnRate(view, &value)
		},
		Messages: previewMessages(view), Selected: selected,
	}
}

type previewBarRow struct {
	ID     charts.ID
	label  i18n.MessageID
	amount string
	tone   charts.Tone
	note   i18n.MessageID
}

func previewBars(view ui.View, selected charts.Selection, symmetric bool, rows []previewBarRow) charts.BarSpec {
	l := view.Language
	spec := charts.BarSpec{
		Label: l.Text(i18n.ReturnContribution), Summary: l.Text(i18n.ChartPreviewHint), Symmetric: symmetric,
		FormatValue: func(v float64) string { return chartAxisNumber(view, v) },
		Messages:    previewMessages(view), Selected: selected,
	}
	for _, row := range rows {
		bar := charts.Bar{ID: row.ID, Label: l.Text(row.label), Tone: row.tone}
		if row.amount == "" {
			bar.Text = l.Text(i18n.OverviewNoValue)
		} else {
			amount := previewDecimal(row.amount)
			bar.Value, bar.Text = previewPlot(amount), previewSigned(view, amount)
		}
		if row.note != i18n.NoMessage {
			bar.Detail = []string{l.Text(row.note)}
		}
		spec.Bars = append(spec.Bars, bar)
	}
	return spec
}

func previewStandardBars(view ui.View, selected charts.Selection) charts.BarSpec {
	return previewBars(view, selected, false, []previewBarRow{
		{ID: "stocks", label: i18n.MarketStocks, amount: "18420.55"},
		{ID: "etfs", label: i18n.MarketETFs, amount: "9310.00"},
		{ID: "funds", label: i18n.MarketFunds, amount: "6002.18"},
		{ID: "bonds", label: i18n.MarketBonds, amount: "-2880.75"},
		{ID: "crypto", label: i18n.MarketCrypto, amount: "12744.90"},
		{ID: "metals", label: i18n.MarketMetals, amount: "1432.10"},
		{ID: "fees", label: i18n.AssetFees, amount: "-1150.00"},
	})
}

// previewEdgeBars centers the axis, with a real zero, a very long name, an
// amount that cannot be quoted and a residual whose tone is chosen by the page.
func previewEdgeBars(view ui.View, selected charts.Selection) charts.BarSpec {
	return previewBars(view, selected, true, []previewBarRow{
		{ID: "money-market", label: i18n.ChartSampleMoneyMarket, amount: "0.00", note: i18n.ChartSampleZeroNote},
		{ID: "unquoted", label: i18n.MarketOther},
		{ID: "share-plan", label: i18n.ChartSampleLongName, amount: "-38210.00"},
		{ID: "adjustment", label: i18n.AssetAdjustment, amount: "-1840.12", tone: charts.ToneWarning},
		{ID: "bonds", label: i18n.MarketBonds, amount: "9610.33"},
	})
}

func previewWaterfallFormat(view ui.View) func(float64) string {
	return func(v float64) string { return chartAxisNumber(view, v) }
}

// previewStandardWaterfall derives the running levels here with decimals, as a
// read model would, and hands every level to the chart.
func previewStandardWaterfall(view ui.View, selected charts.Selection) charts.WaterfallSpec {
	l := view.Language
	type change struct {
		ID     charts.ID
		label  i18n.MessageID
		amount string
		tone   charts.Tone
	}
	changes := []change{
		{ID: "income", label: i18n.AssetIncome, amount: "15250.40"},
		{ID: "spending", label: i18n.AssetSpending, amount: "-60000.00"},
		{ID: "market", label: i18n.AssetMarketMoves, amount: "120000.00"},
		{ID: "adjustment", label: i18n.AssetAdjustment, amount: "1512.35", tone: charts.ToneWarning},
	}
	day := func(month time.Month, d int) string {
		return ui.FormatDay(time.Date(2026, month, d, 0, 0, 0, 0, time.UTC), l, view.DateFormat)
	}
	level := previewDecimal("1000000.00")
	spec := charts.WaterfallSpec{
		Label: l.Text(i18n.AssetBeginning), Summary: l.Text(i18n.ChartPreviewHint),
		FormatValue: previewWaterfallFormat(view), Messages: previewMessages(view), Selected: selected,
		Steps: []charts.WaterfallStep{{ID: "begin", Label: l.Text(i18n.AssetBeginning), Kind: charts.StepTotal,
			To: previewPlot(level), Text: previewMoney(view, level), Detail: []string{day(time.January, 1)}}},
	}
	for _, c := range changes {
		delta := previewDecimal(c.amount)
		next := level.Add(delta)
		spec.Steps = append(spec.Steps, charts.WaterfallStep{ID: c.ID, Label: l.Text(c.label),
			From: previewPlot(level), To: previewPlot(next), Text: previewSigned(view, delta), Tone: c.tone})
		level = next
	}
	spec.Steps = append(spec.Steps, charts.WaterfallStep{ID: "end", Label: l.Text(i18n.AssetEnding), Kind: charts.StepTotal,
		To: previewPlot(level), Text: previewMoney(view, level), Detail: []string{day(time.June, 30)}})
	return spec
}

// previewEdgeWaterfall falls below zero, has a step whose levels are unknown
// and ends on a total that does not reconcile with the last change.
func previewEdgeWaterfall(view ui.View, selected charts.Selection) charts.WaterfallSpec {
	l := view.Language
	begin, loss, gain := previewDecimal("50000.00"), previewDecimal("-180000.00"), previewDecimal("20000.00")
	afterLoss := begin.Add(loss)
	afterGain := afterLoss.Add(gain)
	ending := previewDecimal("-105000.00")
	return charts.WaterfallSpec{
		Label: l.Text(i18n.AssetBeginning), Summary: l.Text(i18n.ChartPreviewHint),
		FormatValue: previewWaterfallFormat(view), Messages: previewMessages(view), Selected: selected,
		Steps: []charts.WaterfallStep{
			{ID: "begin", Label: l.Text(i18n.AssetBeginning), Kind: charts.StepTotal, To: previewPlot(begin), Text: previewMoney(view, begin)},
			{ID: "market", Label: l.Text(i18n.AssetMarketMoves), From: previewPlot(begin), To: previewPlot(afterLoss), Text: previewSigned(view, loss)},
			{ID: "pending", Label: l.Text(i18n.AssetAdjustment), Text: l.Text(i18n.ChartSampleAmountsUnknown)},
			{ID: "income", Label: l.Text(i18n.AssetIncome), From: previewPlot(afterLoss), To: previewPlot(afterGain), Text: previewSigned(view, gain)},
			{ID: "end", Label: l.Text(i18n.AssetEnding), Kind: charts.StepTotal, To: previewPlot(ending), Text: previewMoney(view, ending),
				Detail: []string{fmt.Sprintf(l.Text(i18n.ChartSampleDifference), previewMoney(view, ending.Sub(afterGain)))}},
		},
	}
}

type previewSlice struct {
	ID     charts.ID
	label  i18n.MessageID
	amount string
}

func previewDonut(view ui.View, selected charts.Selection, rows []previewSlice) charts.DonutSpec {
	l := view.Language
	total := decimal.Zero
	for _, row := range rows {
		if amount := previewDecimal(orZero(row.amount)); row.amount != "" && amount.IsPositive() {
			total = total.Add(amount)
		}
	}
	spec := charts.DonutSpec{
		Label: l.Text(i18n.Assets), Summary: l.Text(i18n.ChartPreviewHint),
		CenterValue: previewMoney(view, total), CenterCaption: l.Text(i18n.Assets),
		NotDrawn: l.Text(i18n.ChartNotInRing), ZeroTotal: l.Text(i18n.ChartNothingToDraw),
		Messages: previewMessages(view), Selected: selected,
	}
	for _, row := range rows {
		slice := charts.Slice{ID: row.ID, Label: l.Text(row.label)}
		if row.amount == "" {
			slice.Text = l.Text(i18n.OverviewNoValue)
			spec.Slices = append(spec.Slices, slice)
			continue
		}
		amount := previewDecimal(row.amount)
		slice.Value, slice.Text = previewPlot(amount), previewMoney(view, amount)
		if amount.IsPositive() && total.IsPositive() {
			slice.Share = previewShare(view, amount, total)
		}
		spec.Slices = append(spec.Slices, slice)
	}
	return spec
}

func orZero(amount string) string {
	if amount == "" {
		return "0"
	}
	return amount
}

func previewStandardDonut(view ui.View, selected charts.Selection) charts.DonutSpec {
	return previewDonut(view, selected, []previewSlice{
		{ID: "cash", label: i18n.AnalysisCash, amount: "182400.55"},
		{ID: "stocks", label: i18n.MarketStocks, amount: "412980.10"},
		{ID: "etfs", label: i18n.MarketETFs, amount: "301220.00"},
		{ID: "bonds", label: i18n.MarketBonds, amount: "96300.75"},
		{ID: "crypto", label: i18n.MarketCrypto, amount: "31404.20"},
		{ID: "metals", label: i18n.MarketMetals, amount: "4200.00"},
	})
}

// previewEdgeDonut lists an overdrawn balance, a real zero and an unknown
// amount. None of them gets an arc, and none is turned into a positive value.
func previewEdgeDonut(view ui.View, selected charts.Selection) charts.DonutSpec {
	return previewDonut(view, selected, []previewSlice{
		{ID: "cash", label: i18n.AnalysisCash, amount: "-1250.00"},
		{ID: "stocks", label: i18n.MarketStocks, amount: "412300.00"},
		{ID: "etfs", label: i18n.MarketETFs, amount: "0.00"},
		{ID: "bonds", label: i18n.MarketBonds, amount: ""},
		{ID: "crypto", label: i18n.MarketCrypto, amount: "18900.00"},
	})
}

func previewSankey(view ui.View, selected charts.Selection, rows []chartpreview.Holding) charts.SankeySpec {
	l := view.Language
	spec := charts.SankeySpec{
		Label: l.Text(i18n.Assets), Summary: l.Text(i18n.ChartPreviewHint),
		ColumnTitles: []string{l.Text(i18n.Assets), l.Text(i18n.AnalysisAssetClass), l.Text(i18n.AccountMaintenanceInstitution), l.Text(i18n.AnalysisAccount)},
		Messages:     previewMessages(view), Selected: selected,
	}
	return chartpreview.Sankey(spec, l.Text(i18n.Assets), rows, func(d decimal.Decimal) string {
		return previewMoney(view, d)
	})
}

func previewDenseSankey(view ui.View, selected charts.Selection) charts.SankeySpec {
	l := view.Language
	return previewSankey(view, selected, chartpreview.DenseHoldings([4]string{
		l.Text(i18n.AnalysisCash), l.Text(i18n.MarketStocks), l.Text(i18n.MarketETFs), l.Text(i18n.MarketBonds),
	}))
}

func previewStandardSankey(view ui.View, selected charts.Selection) charts.SankeySpec {
	l := view.Language
	cash := chartpreview.Name{ID: "cash", Label: l.Text(i18n.AnalysisCash)}
	stocks := chartpreview.Name{ID: "stocks", Label: l.Text(i18n.MarketStocks)}
	etfs := chartpreview.Name{ID: "etfs", Label: l.Text(i18n.MarketETFs)}
	bonds := chartpreview.Name{ID: "bonds", Label: l.Text(i18n.MarketBonds)}
	harbor := chartpreview.Name{ID: "harbor", Label: l.Text(i18n.PreviewHarbor)}
	evergreen := chartpreview.Name{ID: "evergreen", Label: l.Text(i18n.PreviewEvergreen)}
	westlake := chartpreview.Name{ID: "westlake", Label: l.Text(i18n.PreviewWestlake)}
	checking := chartpreview.Name{ID: "checking", Label: l.Text(i18n.ChartSampleChecking)}
	savings := chartpreview.Name{ID: "savings", Label: l.Text(i18n.ChartSampleSavings)}
	retirement := chartpreview.Name{ID: "retirement", Label: l.Text(i18n.ChartSampleRetirement)}
	growth := chartpreview.Name{ID: "growth", Label: l.Text(i18n.ChartSampleGrowth)}
	return previewSankey(view, selected, []chartpreview.Holding{
		{Class: cash, Institution: harbor, Account: checking, Amount: "52400.55"},
		{Class: cash, Institution: evergreen, Account: savings, Amount: "98000.00"},
		{Class: stocks, Institution: westlake, Account: growth, Amount: "312980.10"},
		{Class: stocks, Institution: westlake, Account: retirement, Amount: "100000.00"},
		{Class: etfs, Institution: evergreen, Account: growth, Amount: "201220.00"},
		{Class: bonds, Institution: harbor, Account: retirement, Amount: "96300.75"},
	})
}

// previewEdgeSankey adds a very long name, a tiny flow beside large ones, and
// accounts with a zero or an unknown amount that cannot be drawn.
func previewEdgeSankey(view ui.View, selected charts.Selection) charts.SankeySpec {
	l := view.Language
	cash := chartpreview.Name{ID: "cash", Label: l.Text(i18n.AnalysisCash)}
	stocks := chartpreview.Name{ID: "stocks", Label: l.Text(i18n.MarketStocks)}
	crypto := chartpreview.Name{ID: "crypto", Label: l.Text(i18n.MarketCrypto)}
	harbor := chartpreview.Name{ID: "harbor", Label: l.Text(i18n.PreviewHarbor)}
	westlake := chartpreview.Name{ID: "westlake", Label: l.Text(i18n.PreviewWestlake)}
	return previewSankey(view, selected, []chartpreview.Holding{
		{Class: cash, Institution: harbor, Account: chartpreview.Name{ID: "savings", Label: l.Text(i18n.ChartSampleSavings)}, Amount: "845200.00"},
		{Class: cash, Institution: harbor, Account: chartpreview.Name{ID: "checking", Label: l.Text(i18n.ChartSampleChecking)}, Amount: "12.40"},
		{Class: cash, Institution: harbor, Account: chartpreview.Name{ID: "wallet", Label: l.Text(i18n.PreviewCash)}, Amount: "0.00"},
		{Class: stocks, Institution: westlake, Account: chartpreview.Name{ID: "share-plan", Label: l.Text(i18n.ChartSampleLongName)}, Amount: "1250400.75"},
		{Class: stocks, Institution: westlake, Account: chartpreview.Name{ID: "growth", Label: l.Text(i18n.ChartSampleGrowth)}, Amount: ""},
		{Class: crypto, Institution: harbor, Account: chartpreview.Name{ID: "exchange", Label: l.Text(i18n.AccountTypeCrypto)}, Amount: "1.05"},
	})
}
