package desktop

import (
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *returnAnalysisPage) contributionRankingChart(view ui.View, data application.ContributionResult, ranked []application.ContributionRow) layout.Widget {
	spec := p.contributionBars(view, data, ranked)
	chart := &p.output.ranking.chart
	return func(gtx layout.Context) layout.Dimensions {
		if gtx.Enabled() && p.returnFocus == chart {
			p.returnFocus = nil
			gtx.Execute(key.FocusCmd{Tag: chart})
		}
		view.Focus.Watch(gtx, chart)
		return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
			dims, events := chart.Layout(gtx, view, spec)
			if selection := charts.Apply(charts.Selection{}, events); !selection.IsZero() {
				for _, bar := range spec.Bars {
					if bar.ID == selection.Item {
						p.openContribution(string(bar.ID), bar.Label, chart)
						gtx.Execute(op.InvalidateCmd{})
						break
					}
				}
			}
			return dims
		})
	}
}

func (p *returnAnalysisPage) contributionBars(view ui.View, data application.ContributionResult, ranked []application.ContributionRow) charts.BarSpec {
	l := view.Language
	spec := charts.BarSpec{Label: l.Text(i18n.RankingMain), Summary: l.Text(i18n.RankingChartSummary), Symmetric: true,
		Messages: charts.Messages{Empty: l.Text(i18n.RankingNoMatches), Missing: l.Text(i18n.OverviewNoValue), Selected: l.Text(i18n.ChartSelected)}}
	var currency domain.CurrencyCode
	sameCurrency := true
	for _, row := range ranked {
		if row.Amount == nil {
			continue
		}
		if currency == "" {
			currency = row.Amount.Currency()
		}
		sameCurrency = sameCurrency && currency == row.Amount.Currency()
		bar := charts.Bar{ID: charts.ID(row.Key), Label: p.contributionName(l, data.GroupBy, row),
			Value: charts.Num(row.Amount.Amount().InexactFloat64()), Text: returnSignedMoney(view, row.Amount)}
		if data.ReturnType == application.ContributionTotalReturn {
			bar.Detail = append(bar.Detail, l.Text(i18n.ReturnRate)+" · "+returnRate(view, row.Rate))
		}
		if row.Status == domain.CompletenessPartial {
			bar.Detail = append(bar.Detail, l.Text(i18n.OverviewIncomplete))
		}
		spec.Bars = append(spec.Bars, bar)
	}
	if currency != "" && sameCurrency {
		spec.FormatValue = func(tick float64) string { return chartAxisNumber(view, tick) }
	}
	return spec
}
