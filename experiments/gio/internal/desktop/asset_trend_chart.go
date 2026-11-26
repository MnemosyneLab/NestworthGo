package desktop

import (
	"fmt"
	"time"

	"gioui.org/layout"
	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

var assetTrendMetrics = []struct {
	metric application.AssetTrendMetric
	label  i18n.MessageID
}{
	{application.TrendNetWorth, i18n.NetWorth}, {application.TrendAssets, i18n.Assets}, {application.TrendLiabilities, i18n.Liabilities},
	{application.TrendExternalFlow, i18n.AssetExternalFlow}, {application.TrendIncome, i18n.AssetIncome},
	{application.TrendSpending, i18n.AssetSpending}, {application.TrendFees, i18n.AssetFees},
	{application.TrendInvestmentReturn, i18n.AssetTrendInvestmentReturn}, {application.TrendReturnRate, i18n.ReturnRate},
	{application.TrendDividendInterest, i18n.ReturnIncome}, {application.TrendPriceChange, i18n.ReturnPrice},
	{application.TrendFXImpact, i18n.ReturnFX}, {application.TrendFXConversionSpread, i18n.ReturnFXSpread},
	{application.TrendNetChange, i18n.AssetNetChange}, {application.TrendResidual, i18n.AssetResidual},
}

func assetTrendChoices(l i18n.Language) []ui.Choice {
	choices := make([]ui.Choice, len(assetTrendMetrics))
	for i, option := range assetTrendMetrics {
		choices[i] = ui.Choice{ID: string(option.metric), Label: l.Text(option.label)}
	}
	return choices
}

func assetTrendTitle(l i18n.Language, metric application.AssetTrendMetric) string {
	for _, option := range assetTrendMetrics {
		if option.metric == metric {
			return l.Text(option.label)
		}
	}
	return l.Text(i18n.AssetTrend)
}

type assetTrendPeriod struct {
	key string
	day time.Time
}

func assetTrendPeriods(from, to domain.LocalDate, interval application.AssetTrendGranularity) []assetTrendPeriod {
	start, validStart := ui.ParseCivilDay(from)
	end, validEnd := ui.ParseCivilDay(to)
	if !validStart || !validEnd || end.Before(start) {
		return nil
	}
	day, months, days := start, 0, 1
	keyLayout := time.DateOnly
	switch interval {
	case application.TrendWeek:
		offset := (int(start.Weekday()) + 6) % 7
		day, days = start.AddDate(0, 0, -offset), 7
	case application.TrendMonth:
		day, months, days = start.AddDate(0, 0, 1-start.Day()), 1, 0
		keyLayout = "2006-01"
	}
	var periods []assetTrendPeriod
	for !day.After(end) {
		plotDay := day
		if plotDay.Before(start) {
			plotDay = start
		}
		periods = append(periods, assetTrendPeriod{key: day.Format(keyLayout), day: plotDay})
		day = day.AddDate(0, months, days)
	}
	return periods
}

func assetTrendLine(view ui.View, trend application.AssetTrendResult, query domain.AnalysisQuery, metric application.AssetTrendMetric, interval application.AssetTrendGranularity) (charts.LineSpec, int) {
	l := view.Language
	label := assetTrendTitle(l, metric)
	series := charts.LineSeries{ID: charts.ID(metric), Name: label, Color: view.Colors.Accent}
	points := make(map[string]application.AssetTrendPoint, len(trend.Points))
	currency := domain.CurrencyCode("")
	if trend.Summary != nil {
		currency = trend.Summary.Currency()
	}
	for _, point := range trend.Points {
		points[point.Period] = point
		if point.DisplayValuation != nil && currency == "" {
			currency = point.DisplayValuation.Currency
		}
		if point.Value != nil && currency == "" {
			currency = point.Value.Currency()
		}
	}
	missing := 0
	for _, period := range assetTrendPeriods(query.From, query.To, interval) {
		point := assetTrendChartPoint(view, metric, period, points[period.key])
		if !point.Value.Valid() {
			missing++
		}
		series.Points = append(series.Points, point)
	}
	start, _ := ui.ParseCivilDay(query.From)
	end, _ := ui.ParseCivilDay(query.To)
	return charts.LineSpec{
		Label: label, Summary: l.Text(i18n.AssetTrendChartHelp), Series: []charts.LineSeries{series}, From: start, To: end,
		FormatTime: func(day time.Time) string { return ui.FormatDay(day, l, view.DateFormat) },
		FormatValue: func(value float64) string {
			amount := decimal.NewFromFloat(value)
			if metric == application.TrendReturnRate {
				return returnRate(view, &amount)
			}
			return valuationMoney(view, domain.MoneyView{Amount: amount.String(), Currency: currency}, false)
		},
		Messages: charts.Messages{Empty: l.Text(i18n.AssetNoData), Missing: l.Text(i18n.OverviewNoValue), Selected: l.Text(i18n.ChartSelected)},
	}, missing
}

func assetTrendChartPoint(view ui.View, metric application.AssetTrendMetric, period assetTrendPeriod, value application.AssetTrendPoint) charts.LinePoint {
	l := view.Language
	point := charts.LinePoint{ID: charts.ID(period.key), Time: period.day}
	debts := metric == application.TrendLiabilities
	level := metric == application.TrendNetWorth || metric == application.TrendAssets || debts
	var money *domain.MoneyView
	if value.Value != nil {
		money = &domain.MoneyView{Currency: value.Value.Currency(), Amount: value.Value.CanonicalAmount()}
	}
	if display := value.DisplayValuation; level && display != nil && display.Quality.Availability != "" {
		quality := display.Quality
		money, point.Provisional = display.Total, quality.Coverage == domain.CoverageProvisional
		if money == nil && display.KnownSubtotal != nil {
			label := i18n.OverviewKnownSubtotal
			if metric == application.TrendNetWorth {
				label = i18n.OverviewKnownNet
			}
			point.Detail = append(point.Detail, l.Text(label)+" · "+valuationMoney(view, *display.KnownSubtotal, debts))
		}
		if point.Provisional {
			point.Detail = append(point.Detail, l.Text(i18n.WealthHistoryProvisional))
		}
		if quality.UnpricedCount > 0 {
			point.Detail = append(point.Detail, fmt.Sprintf(l.Text(i18n.WealthHistoryMissing), quality.UnpricedCount))
		}
		if quality.OldestMarketDate != nil {
			date := wealthDay(view, *quality.OldestMarketDate)
			if quality.NewestMarketDate != nil && *quality.NewestMarketDate != *quality.OldestMarketDate {
				date += " – " + wealthDay(view, *quality.NewestMarketDate)
			}
			point.Detail = append(point.Detail, fmt.Sprintf(l.Text(i18n.OverviewQuoteDates), date))
		}
		if quality.CarriedCount > 0 || quality.StaleCount > 0 {
			point.Detail = append(point.Detail, fmt.Sprintf(l.Text(i18n.WealthHistoryQuoteAge), quality.CarriedCount, quality.StaleCount))
		}
	}
	switch {
	case metric == application.TrendReturnRate:
		if value.Rate != nil {
			point.Value, point.Text = charts.Num(value.Rate.InexactFloat64()), returnRate(view, value.Rate)
		}
		if value.Coverage.TotalDays > 0 {
			point.Detail = append(point.Detail, returnCoverage(l, value.Coverage, application.ReturnDayCoverage{}))
		}
	case money != nil:
		if amount, err := decimal.NewFromString(money.Amount); err == nil {
			if debts && amount.IsPositive() {
				amount = amount.Neg()
			}
			point.Value, point.Text = charts.Num(amount.InexactFloat64()), valuationMoney(view, *money, debts)
		}
	}
	if !point.Value.Valid() {
		point.Detail = append(point.Detail, l.Text(i18n.AssetTrendMissingValue))
	}
	return point
}

func (p *assetChangesPage) assetTrendRows(view ui.View, data application.AssetTrendResult, query domain.AnalysisQuery) []ui.ColumnChild {
	l := view.Language
	metric := p.trendMetric()
	var rows []ui.ColumnChild
	if data.ValueChange != nil {
		rows = append(rows, ui.Rigid(assetSummary(view, *data.ValueChange, metric == application.TrendLiabilities)))
	}
	rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions { return p.options(gtx, view) }))
	if !data.Available {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.AssetNoData), view.Colors.SecondaryInk)))
	}
	if data.ValuationForced != nil {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.AnalysisBaseFallback), view.Colors.SecondaryInk)))
	}
	if len(data.Points) == 0 && data.ValueChange == nil {
		return rows
	}
	spec, missing := assetTrendLine(view, data, query, metric, p.trendInterval())
	if missing > 0 {
		rows = append(rows, ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.AssetTrendMissingPeriods), missing), view.Colors.SecondaryInk)))
	}
	rows = append(rows, ui.Rigid(analysisLineCard(view, &p.trendChart, spec, &p.trendSelected)))
	table := spec.Rows()
	rows = append(rows, ui.Fill(func(gtx layout.Context) layout.Dimensions {
		return analysisResultList(gtx, view, &p.list, l.Text(i18n.AssetTrendTable), len(table), func(gtx layout.Context, index int) layout.Dimensions {
			return analysisTableRow(view, table[index])(gtx)
		})
	}))
	if data.ValueChange == nil {
		value := returnMoney(view, data.Summary)
		if metric == application.TrendReturnRate {
			value = returnRate(view, data.Rate)
			rows = append(rows, ui.Rigid(view.Message(returnCoverage(l, data.Coverage, data.ReturnDayCoverage), view.Colors.SecondaryInk)))
		}
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.AssetTrendPeriodValue)+" · "+value, view.Colors.Ink)))
	}
	return rows
}
