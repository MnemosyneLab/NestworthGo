package desktop

import (
	"time"

	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func returnTrendLine(view ui.View, trend application.ReturnTrendResult, from, to domain.LocalDate) charts.LineSpec {
	l := view.Language
	label := i18n.ReturnCumulative
	switch trend.Display {
	case application.ReturnTrendLinkedRate:
		label = i18n.ReturnLinked
	case application.ReturnTrendPeriodReturnAmount:
		label = i18n.ReturnPeriodAmount
	}
	series := charts.LineSeries{ID: charts.ID(trend.Display), Name: l.Text(label), Color: view.Colors.Accent}
	start, _ := ui.ParseCivilDay(from)
	end, _ := ui.ParseCivilDay(to)
	currency := domain.CurrencyCode("")
	if trend.Amount != nil {
		currency = trend.Amount.Currency()
	}
	for _, point := range trend.Points {
		day, valid := ui.ParseCivilDay(point.Date)
		if !valid {
			continue
		}
		if currency == "" && point.Value != nil {
			currency = point.Value.Currency()
		}
		series.Points = append(series.Points, returnTrendObservation(view, trend.Display, point, day))
	}
	baseline := start.AddDate(0, 0, -1)
	if trend.Display == application.ReturnTrendCumulativeAmount && baseline.Year() > 0 && currency != "" {
		for _, observation := range series.Points {
			if observation.Value.Valid() {
				start = baseline
				zero := valuationMoney(view, domain.MoneyView{Amount: "0", Currency: currency}, false)
				series.Points = append([]charts.LinePoint{{ID: charts.ID(start.Format(time.DateOnly)), Time: start,
					Value: charts.Num(0), Text: zero, Detail: []string{l.Text(i18n.ReturnTrendBaseline)}}}, series.Points...)
				break
			}
		}
	}
	return charts.LineSpec{
		Label: l.Text(label), Summary: l.Text(i18n.ReturnTrendChartSummary), Series: []charts.LineSeries{series},
		From: start, To: end, IncludeZero: trend.Display != application.ReturnTrendLinkedRate,
		FormatTime: func(day time.Time) string { return ui.FormatDay(day, l, view.DateFormat) },
		FormatValue: func(tick float64) string {
			number := decimal.NewFromFloat(tick)
			if trend.Display == application.ReturnTrendLinkedRate {
				return returnRate(view, &number)
			}
			return chartAxisNumber(view, tick)
		},
		Messages: charts.Messages{Empty: l.Text(i18n.ReturnInsufficient), Missing: l.Text(i18n.OverviewNoValue), Selected: l.Text(i18n.ChartSelected)},
	}
}

func returnTrendObservation(view ui.View, display application.ReturnTrendDisplay, point application.ReturnTrendPoint, day time.Time) charts.LinePoint {
	l := view.Language
	observation := charts.LinePoint{ID: charts.ID(point.Date), Time: day,
		Detail:      []string{returnCoverage(l, point.Coverage, application.ReturnDayCoverage{})},
		Provisional: point.Status == domain.CompletenessPartial}
	switch {
	case point.ValuationMissing:
		observation.Detail = append(observation.Detail, l.Text(i18n.ReturnTrendValuationGap))
	case display == application.ReturnTrendLinkedRate && point.Rate != nil:
		observation.Value, observation.Text = charts.Num(point.Rate.InexactFloat64()), returnRate(view, point.Rate)
	case display == application.ReturnTrendLinkedRate && point.Amount != nil:
		observation.Detail = append(observation.Detail, l.Text(i18n.ReturnTrendRateGap))
	case display != application.ReturnTrendLinkedRate && point.Value != nil && point.Amount != nil:
		// The cumulative service value carries across missing observations. A
		// missing daily amount still interrupts the plotted line and table.
		observation.Value, observation.Text = charts.Num(point.Value.Amount().InexactFloat64()), returnMoney(view, point.Value)
	default:
		observation.Detail = append(observation.Detail, l.Text(i18n.ReturnTrendGap))
	}
	return observation
}
