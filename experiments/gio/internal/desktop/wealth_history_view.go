package desktop

import (
	"fmt"
	"image"
	"slices"
	"strings"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *wealthHistoryPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	p.update()
	l := view.Language
	if p.origin == nil {
		status, ink := i18n.WealthHistoryNoOrigin, view.Colors.SecondaryInk
		switch {
		case p.originRead != nil:
			status = i18n.WealthHistoryLoading
		case p.err != nil:
			status, ink = i18n.WealthHistoryFailed, view.Colors.ErrorInk
		}
		return ui.Column{Gap: 10}.Layout(gtx,
			ui.Rigid(view.Message(l.Text(status), ink)),
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				label := i18n.HistoryRefresh
				if p.err != nil {
					label = i18n.HistoryRetry
				}
				if p.originRead != nil {
					gtx = gtx.Disabled()
				} else if p.load.Clicked(gtx) {
					p.readOrigin()
					gtx.Execute(op.InvalidateCmd{})
				}
				view.Focus.Watch(gtx, &p.load)
				return view.Button(&p.load, l.Text(label))(gtx)
			}),
		)
	}
	now := p.clock()
	if p.draft == (wealthDateRange{}) {
		return view.Message(l.Text(i18n.WealthHistoryNoClosedDays), view.Colors.SecondaryInk)(gtx)
	}
	if p.allocationTab.Clicked(gtx) && p.lens != wealthAllocationLens {
		p.returnToAllocation(view)
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.accountsTab.Clicked(gtx) && p.lens != wealthAccountsLens {
		p.selectEndpointLens(wealthAccountsLens)
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.changeTab.Clicked(gtx) && p.lens != wealthChangeLens && p.accounts.current != nil && p.accounts.current.Right != nil {
		p.selectEndpointLens(wealthChangeLens)
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.trendTab.Clicked(gtx) && p.lens != wealthTrendLens {
		p.selectTrend()
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.lens != wealthTrendLens {
		return p.layoutWealthAccounts(gtx, view, now)
	}
	valid := p.origin.permits(p.draft, now)
	if p.load.Clicked(gtx) && valid && (p.seriesRead == nil || p.requested != p.draft) {
		p.readSeries(p.draft)
		gtx.Execute(op.InvalidateCmd{})
	}
	children := []ui.ColumnChild{
		ui.Rigid(func(gtx layout.Context) layout.Dimensions { return p.dateFields(gtx, view, now) }),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := i18n.WealthHistoryLoad
			switch {
			case p.err != nil && p.draft == p.requested:
				label = i18n.HistoryRetry
			case p.current != nil && p.draft == p.current.rangeRead:
				label = i18n.HistoryRefresh
			}
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(12)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if !valid {
						gtx = gtx.Disabled()
					}
					view.Focus.Watch(gtx, &p.load)
					return view.Button(&p.load, l.Text(label))(gtx)
				}),
				layout.Flexed(1, view.Message(l.Text(i18n.WealthHistoryRangeHint), view.Colors.SecondaryInk)),
			)
		}),
	}
	if p.current != nil {
		series := p.current.series
		caption := fmt.Sprintf(l.Text(i18n.WealthHistoryCapture), wealthDay(view, series.From),
			wealthDay(view, series.To), series.Overview.Left.Currency, series.Overview.Timezone)
		children = append(children, ui.Rigid(view.Body(ui.TextLabel, caption, view.Colors.Ink)))
		if captured, err := time.Parse(time.RFC3339Nano, series.Overview.CapturedAt); err == nil {
			children = append(children, ui.Rigid(view.Message(valuationReadTime(view, captured), view.Colors.SecondaryInk)))
		}
	}
	switch {
	case p.seriesRead != nil:
		children = append(children, ui.Rigid(view.Message(l.Text(i18n.WealthHistoryLoading), view.Colors.SecondaryInk)))
	case p.err != nil:
		id := i18n.WealthHistoryFailed
		if wealthHistoryBudgetExceeded(p.err) {
			id = i18n.WealthHistoryBudget
		}
		children = append(children, ui.Rigid(view.Message(l.Text(id), view.Colors.ErrorInk)))
		if p.current != nil {
			children = append(children, ui.Rigid(view.Message(l.Text(i18n.WealthHistoryRetained), view.Colors.SecondaryInk)))
		}
	}
	children = append(children, ui.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutWealthTabs(gtx, view) }))
	if p.current != nil {
		if len(p.current.rows) == 0 {
			children = append(children, ui.Rigid(view.Message(l.Text(i18n.WealthHistoryEmpty), view.Colors.SecondaryInk)))
		} else {
			children = append(children, p.wealthTrendControls(gtx, view)...)
			children = append(children, ui.Fill(func(gtx layout.Context) layout.Dimensions { return p.table(gtx, view) }))
		}
	}
	return ui.Column{Gap: 10}.Layout(gtx, children...)
}

func (p *wealthHistoryPage) dateFields(gtx layout.Context, view ui.View, now time.Time) layout.Dimensions {
	last := p.origin.lastClosed(now).Format(time.DateOnly)
	options := ui.DateOptions{Today: last, Min: p.origin.first, Max: last, Format: view.DateFormat, WeekStart: time.Monday}
	if view.WeekStart == "sunday" {
		options.WeekStart = time.Sunday
	}
	return (ui.Grid{MinColumn: 150, MaxColumns: 2, Gap: 10}).Layout(gtx, view, 2,
		func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
			picker, label, value := &p.from, i18n.HistoryFilterFrom, &p.draft.from
			if i == 1 {
				picker, label, value = &p.to, i18n.HistoryFilterTo, &p.draft.to
			}
			picker.Layout(gtx, view, ui.PickerField{Label: label, Origin: view.Origin}, options, *value, func(day string) {
				*value = day
				gtx.Execute(op.InvalidateCmd{})
			})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
		})
}

func (p *wealthHistoryPage) table(gtx layout.Context, view ui.View) layout.Dimensions {
	border := view.Colors.Divider
	if gtx.Focused(&p.list) {
		border = view.Colors.Accent
	}
	rows := p.current.rows
	return view.Panel(gtx, border, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(8).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
			view.Focus.Watch(gtx, &p.list)
			event.Op(gtx.Ops, &p.list)
			for _, pressed := range ui.PressedKeys(gtx, &p.list, key.NameUpArrow, key.NameDownArrow,
				key.NamePageUp, key.NamePageDown, key.NameHome, key.NameEnd) {
				step := float32(max(1, p.list.Position.Count-1))
				switch pressed.Name {
				case key.NameUpArrow:
					p.list.ScrollBy(-1)
				case key.NameDownArrow:
					p.list.ScrollBy(1)
				case key.NamePageUp:
					p.list.ScrollBy(-step)
				case key.NamePageDown:
					p.list.ScrollBy(step)
				case key.NameHome:
					p.list.ScrollTo(0)
				case key.NameEnd:
					p.list.ScrollTo(len(rows) - 1)
				}
			}
			return accessibility.List(gtx, &p.list, view.Language.Text(i18n.WealthHistoryTable), func(gtx layout.Context) layout.Dimensions {
				return view.List(&p.list).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
					row := rows[i]
					tag := any(row.point)
					if row.missing != nil {
						tag = row.missing
					}
					return accessibility.Row(gtx, tag, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: 6, Bottom: 6, Left: 8, Right: 8}.Layout(gtx,
							func(gtx layout.Context) layout.Dimensions { return p.wealthRow(gtx, view, row) })
					})
				})
			})
		})
	})
}

func (p *wealthHistoryPage) wealthRow(gtx layout.Context, view ui.View, row wealthHistoryRow) layout.Dimensions {
	l, point := view.Language, row.point
	date := wealthDay(view, point.Date)
	if row.missing != nil {
		return view.Message(date+" · "+wealthMissing(l, *row.missing), view.Colors.SecondaryInk)(gtx)
	}
	if !point.Available {
		message := i18n.OverviewNoValue
		if point.GapReason == "nonexistent_civil_date" {
			message = i18n.WealthHistorySkippedDate
		}
		return view.Message(date+" · "+l.Text(message), view.Colors.SecondaryInk)(gtx)
	}
	values := point.DisplayValuation
	button := portfolioButton(p.trend.days, point.Date)
	selection := charts.Selection{Series: wealthTrendSeries, Item: charts.ID(point.Date)}
	if button.Clicked(gtx) {
		p.trend.selected = selection
		gtx.Execute(op.InvalidateCmd{})
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(5)}.Layout(gtx,
		layout.Rigid(analysisToggle(view, button, date, p.trend.selected == selection)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return (ui.Grid{MinColumn: 150, MaxColumns: 3, Gap: 8, Balanced: true}).Layout(gtx, view, 3,
				func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
					labels := [...]i18n.MessageID{i18n.NetWorth, i18n.Assets, i18n.Liabilities}
					amounts := [...]domain.ValuationAmount{values.NetWorth, values.Assets, values.Liabilities}
					return view.Message(wealthAmount(view, labels[i], amounts[i]), view.Colors.Ink)(gtx)
				})
		}),
		layout.Rigid(view.Message(wealthCoverage(view, point), view.Colors.SecondaryInk)),
	)
}

func wealthAmount(view ui.View, label i18n.MessageID, value domain.ValuationAmount) string {
	name, display := wealthAmountParts(view, label, value)
	return name + " · " + display
}

// wealthAmountParts splits wealthAmount into its caption and the figure.
func wealthAmountParts(view ui.View, label i18n.MessageID, value domain.ValuationAmount) (name, display string) {
	l := view.Language
	name = l.Text(label)
	if qualifier := wealthAmountQualifier(label, value); qualifier != i18n.NoMessage {
		name += " · " + l.Text(qualifier)
	}
	money := value.Total
	if money == nil {
		money = value.KnownSubtotal
	}
	display = l.Text(i18n.OverviewNoValue)
	if money != nil {
		display = valuationMoney(view, *money, label == i18n.Liabilities)
	}
	return name, display
}

func wealthAmountQualifier(label i18n.MessageID, value domain.ValuationAmount) i18n.MessageID {
	switch {
	case value.Total == nil && value.KnownSubtotal != nil:
		if label == i18n.NetWorth {
			return i18n.OverviewKnownNet
		}
		return i18n.OverviewKnownSubtotal
	case value.Total != nil && value.Quality.Coverage == domain.CoverageProvisional:
		return i18n.WealthHistoryProvisional
	}
	return i18n.NoMessage
}

func wealthCoverage(view ui.View, point *application.HistoricalSeriesPoint) string {
	return wealthQualityCoverage(view, point, point.DisplayValuation.NetWorth.Quality)
}

func wealthQualityCoverage(view ui.View, point *application.HistoricalSeriesPoint, q domain.ValuationQuality) string {
	l := view.Language
	a, d := point.AssetCoverage, point.LiabilityCoverage
	parts := []string{fmt.Sprintf(l.Text(i18n.WealthHistoryCoverage), a.Known, a.Included, d.Known, d.Included)}
	if len(point.Missing) > 0 {
		parts = append(parts, fmt.Sprintf(l.Text(i18n.WealthHistoryMissing), len(point.Missing)))
	}
	if q.OldestMarketDate != nil {
		dates := wealthDay(view, *q.OldestMarketDate)
		if q.NewestMarketDate != nil && *q.NewestMarketDate != *q.OldestMarketDate {
			dates += " – " + wealthDay(view, *q.NewestMarketDate)
		}
		parts = append(parts, fmt.Sprintf(l.Text(i18n.OverviewQuoteDates), dates))
	}
	if q.CarriedCount > 0 || q.StaleCount > 0 {
		parts = append(parts, fmt.Sprintf(l.Text(i18n.WealthHistoryQuoteAge), q.CarriedCount, q.StaleCount))
	}
	if slices.Contains(q.Reasons, domain.ReasonHistoryUnverified) {
		parts = append(parts, l.Text(i18n.OverviewHistoryUnverified))
	}
	if slices.Contains(q.Reasons, domain.ReasonEvidenceUnavailable) {
		parts = append(parts, l.Text(i18n.OverviewEvidenceUnavailable))
	}
	if point.ScopeChanged {
		parts = append(parts, l.Text(i18n.WealthHistoryScopeChanged))
	}
	if point.BasisChanged {
		parts = append(parts, l.Text(i18n.WealthHistoryBasisChanged))
	}
	return strings.Join(parts, " · ")
}

func wealthMissing(l i18n.Language, missing application.HistoricalSeriesMissing) string {
	parts := []string{missing.Name}
	for _, reason := range missing.Reasons {
		label, ok := overviewMissing[domain.MissingInputKind(reason)]
		if reason == "funding_cash_after_product_start" {
			label, ok = i18n.WealthHistoryFundingGap, true
		}
		if !ok {
			label = i18n.OverviewEvidenceUnavailable
		}
		parts = append(parts, l.Text(label))
	}
	return strings.Join(parts, " · ")
}

func wealthDay(view ui.View, value string) string {
	if day, ok := ui.ParseCivilDay(value); ok {
		return ui.FormatDay(day, view.Language, view.DateFormat)
	}
	return value
}
