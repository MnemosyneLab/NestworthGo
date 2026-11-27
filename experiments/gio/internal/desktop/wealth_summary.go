package desktop

import (
	"time"

	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func wealthAccountSummary(view ui.View, result application.HistoricalOverviewResult) []ui.ColumnChild {
	l := view.Language
	headline := result.Left
	var headlineDate string
	if result.Right != nil {
		headline = *result.Right
		headlineDate = wealthEndpointLabel(l, view, headline, 1)
	}
	baselineDate := wealthEndpointLabel(l, view, result.Left, 0)
	metrics := [...]struct {
		label      i18n.MessageID
		main, base domain.ValuationAmount
		change     *string
	}{
		{label: i18n.NetWorth, main: headline.DisplayValuation.NetWorth, base: result.Left.DisplayValuation.NetWorth},
		{label: i18n.Assets, main: headline.DisplayValuation.Assets, base: result.Left.DisplayValuation.Assets},
		{label: i18n.Liabilities, main: headline.DisplayValuation.Liabilities, base: result.Left.DisplayValuation.Liabilities},
	}
	change := result.EstimatedChange
	if change == nil {
		change = result.Change
	}
	if change != nil && result.Right != nil && headline.Currency == result.Left.Currency {
		metrics[0].change, metrics[1].change, metrics[2].change = change.NetWorth, change.Assets, change.Liabilities
	}
	rows := []ui.ColumnChild{ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		return (ui.Grid{MinColumn: 178, MaxColumns: 3, Gap: ui.Space8}).Layout(gtx, view, len(metrics),
			func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
				metric := metrics[i]
				return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
					main := []ui.ColumnChild{
						ui.Rigid(view.Body(ui.TextCaptionStrong, l.Text(metric.label), view.Colors.SecondaryInk)),
					}
					main = append(main, wealthSummaryFigure(view, metric.label, headlineDate, metric.main, ui.TextMoneyLG)...)
					if metric.change != nil {
						value := wealthExactMoney(view, metric.change, headline.Currency, metric.label == i18n.Liabilities, true)
						main = append(main, ui.Rigid(view.Body(ui.TextCaptionStrong, l.Text(i18n.WealthAccountDelta)+" · "+value, view.Colors.SecondaryInk)))
					}
					if result.Right != nil {
						main = append(main, ui.Rigid(view.Hairline))
						main = append(main, wealthSummaryFigure(view, metric.label, baselineDate, metric.base, ui.TextLabel)...)
					}
					return (ui.Column{Gap: ui.Space4}).Layout(gtx, main...)
				})
			})
	})}
	rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		states := []application.HistoricalOverviewState{result.Left}
		if result.Right != nil {
			states = append(states, *result.Right)
		}
		statuses := make([]layout.Widget, 0, len(states)+1)
		for i, state := range states {
			status, tone := wealthEndpointCoverage(state)
			label := wealthSummaryDate(view, state, i, result.Right != nil) + " · " + l.Text(status)
			statuses = append(statuses, func(gtx layout.Context) layout.Dimensions { return view.Badge(gtx, tone, label) })
		}
		if result.Right != nil && result.Left.Currency != result.Right.Currency {
			statuses = append(statuses, view.Body(ui.TextCaption, l.Text(i18n.WealthAllocationCurrencyChanged), view.Colors.SecondaryInk))
		}
		return (ui.Wrap{Gap: ui.Space6, Middle: true}).Layout(gtx, len(statuses), func(gtx layout.Context, i int) layout.Dimensions {
			return statuses[i](gtx)
		})
	}))
	if captured, err := time.Parse(time.RFC3339Nano, result.CapturedAt); err == nil {
		rows = append(rows, ui.Rigid(view.Body(ui.TextCaption, valuationReadTime(view, captured), view.Colors.SecondaryInk)))
	}
	return rows
}

func wealthSummaryFigure(view ui.View, label i18n.MessageID, date string, value domain.ValuationAmount, style ui.TextStyle) []ui.ColumnChild {
	_, display := wealthAmountParts(view, label, value)
	if qualifier := wealthAmountQualifier(label, value); qualifier != i18n.NoMessage {
		if date != "" {
			date += " · "
		}
		date += view.Language.Text(qualifier)
	}
	var rows []ui.ColumnChild
	if date != "" {
		rows = append(rows, ui.Rigid(view.Body(ui.TextCaption, date, view.Colors.SecondaryInk)))
	}
	return append(rows, ui.Rigid(view.Body(style, display, view.Colors.Ink)))
}

func wealthSummaryDate(view ui.View, state application.HistoricalOverviewState, side int, compared bool) string {
	if compared {
		return wealthEndpointLabel(view.Language, view, state, side)
	}
	return wealthDay(view, state.Date)
}

func wealthEndpointCoverage(state application.HistoricalOverviewState) (i18n.MessageID, ui.BadgeTone) {
	values := state.DisplayValuation
	estimated := !state.Complete
	for _, value := range [...]domain.ValuationAmount{values.NetWorth, values.Assets, values.Liabilities} {
		if value.Total == nil || value.Quality.Availability != domain.ValuationFull {
			return i18n.WealthCoveragePartial, ui.BadgeWarning
		}
		q := value.Quality
		estimated = estimated || q.CarriedCount > 0 || q.Coverage == domain.CoverageProvisional || q.Coverage == domain.CoverageUnavailable
	}
	if estimated {
		return i18n.WealthCoverageEstimated, ui.BadgeWarning
	}
	return i18n.WealthCoverageComplete, ui.BadgeSuccess
}
