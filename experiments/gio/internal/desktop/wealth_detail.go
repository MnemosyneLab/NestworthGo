package desktop

import (
	"fmt"
	"strings"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type wealthDetail struct {
	analysisSheet
	result      *application.HistoricalOverviewResult
	row         *application.HistoricalOverviewRow
	trigger     event.Tag
	links       map[string]*widget.Clickable
	focusAction bool
	component   string
}

func (p *wealthHistoryPage) watchWealthDetailFocus(gtx layout.Context, view ui.View, tag event.Tag) {
	view.Focus.Watch(gtx, tag)
	if gtx.Enabled() && p.detailFocus == tag {
		p.detailFocus = nil
		gtx.Execute(key.FocusCmd{Tag: tag})
	}
}

func (p *wealthHistoryPage) openWealthDetail(result *application.HistoricalOverviewResult, row *application.HistoricalOverviewRow, trigger event.Tag) {
	p.detail = &wealthDetail{analysisSheet: newAnalysisSheet(), result: result, row: row, trigger: trigger, links: make(map[string]*widget.Clickable)}
}

func (p *wealthHistoryPage) wealthDetailButton(view ui.View, result *application.HistoricalOverviewResult, row application.HistoricalOverviewRow, label string) layout.Widget {
	if p.accounts.details == nil {
		p.accounts.details = make(map[string]*widget.Clickable)
	}
	button := portfolioButton(p.accounts.details, row.Key)
	return func(gtx layout.Context) layout.Dimensions {
		p.watchWealthDetailFocus(gtx, view, button)
		if button.Clicked(gtx) {
			p.openWealthDetail(result, &row, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		return view.ButtonOf(button, ui.ButtonSpec{Title: label, Variant: ui.ButtonOutline, Size: ui.ButtonSmall})(gtx)
	}
}

func (p *wealthHistoryPage) reconciliationSummary(view ui.View, result *application.HistoricalOverviewResult) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		l, r := view.Language, result.Reconciliation
		button := &p.change.reconcile
		p.watchWealthDetailFocus(gtx, view, button)
		if button.Clicked(gtx) {
			p.openWealthDetail(result, nil, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				totals := reconciliationTotals(view, r)
				return (ui.Column{Gap: 6}).Layout(gtx,
					ui.Rigid(view.Body(ui.TextLabel, l.Text(i18n.WealthBridgeTitle), view.Colors.Ink)),
					ui.Rigid(func(gtx layout.Context) layout.Dimensions {
						return (ui.Grid{MinColumn: 160, MaxColumns: 3, Gap: 8}).Layout(gtx, view, len(totals),
							func(gtx layout.Context, _ ui.View, i int) layout.Dimensions { return totals[i](gtx) })
					}),
					ui.Rigid(view.Message(l.Text(wealthBridgeStatus(r.Status)), view.Colors.SecondaryInk)),
					ui.Rigid(returnMonthButton(view, button, i18n.WealthBridgeEvidence, nil, true)))
			})
		})
	}
}

func reconciliationTotals(view ui.View, r *application.HistoricalReconciliation) []layout.Widget {
	l := view.Language
	values := []*string{r.Total, r.Supported, r.Residual}
	return []layout.Widget{
		view.Message(l.Text(i18n.WealthAccountNetChange)+" · "+wealthExactMoney(view, values[0], r.Currency, false, true), view.Colors.Ink),
		view.Message(l.Text(i18n.WealthBridgeSupported)+" · "+wealthExactMoney(view, values[1], r.Currency, false, true), view.Colors.Ink),
		view.Message(l.Text(i18n.WealthBridgeResidual)+" · "+wealthExactMoney(view, values[2], r.Currency, false, true), view.Colors.Ink),
	}
}

func (p *wealthHistoryPage) layoutWealthDetail(gtx layout.Context, view ui.View) {
	s := p.detail
	dismiss := s.dismissed(gtx)
	for _, tag := range s.links {
		dismiss = len(ui.PressedKeys(gtx, tag, key.NameEscape)) > 0 || dismiss
	}
	if dismiss {
		p.detail, p.detailFocus = nil, s.trigger
		gtx.Execute(op.InvalidateCmd{})
		return
	}
	l := view.Language
	title, action := l.Text(i18n.WealthBridgeTitle), i18n.NoMessage
	items := func() []layout.Widget { return wealthBridgeEvidence(view, s) }
	if s.row != nil {
		title = wealthRowName(l, *s.row)
		items = func() []layout.Widget { return p.wealthRowEvidence(view, s) }
		if s.row.AccountID != "" && p.openHistory != nil {
			action = i18n.ContributionOpenHistory
			if s.focusAction {
				s.focusAction = false
				gtx.Execute(key.FocusCmd{Tag: &s.action})
			}
			if s.action.Clicked(gtx) {
				s.focusAction = true
				p.detail, p.returnDetail = nil, s
				p.openHistory(wealthHistoryHandoff(s.result, *s.row), s.trigger)
				gtx.Execute(op.InvalidateCmd{})
				return
			}
		}
	}
	s.layout(gtx, view, title, func(gtx layout.Context) layout.Dimensions {
		rows := items()
		return view.List(&s.list).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
			return (layout.Inset{Top: 4, Bottom: 4}).Layout(gtx, rows[i])
		})
	}, action)
}

func wealthHistoryHandoff(result *application.HistoricalOverviewResult, row application.HistoricalOverviewRow) history.Filters {
	from, to := result.Left.Date, result.Left.Date
	if result.Right != nil {
		if result.Right.Current {
			to = result.LastClosedDate
		} else {
			from, to = min(from, result.Right.Date), max(to, result.Right.Date)
		}
	}
	return history.Filters{Account: domain.AccountID(row.AccountID), Instrument: domain.InstrumentID(row.InstrumentID), From: from, To: to}
}

func (p *wealthHistoryPage) wealthRowEvidence(view ui.View, sheet *wealthDetail) []layout.Widget {
	result, row, l := sheet.result, *sheet.row, view.Language
	if location, err := time.LoadLocation(result.Timezone); err == nil {
		view.TimeLocation = location
	}
	liability := row.Role == string(domain.RoleLiability)
	rows := []layout.Widget{view.Message(l.Text(i18n.WealthChangeBasis)+" · "+result.Timezone, view.Colors.SecondaryInk)}
	if captured, err := time.Parse(time.RFC3339Nano, result.CapturedAt); err == nil {
		rows = append(rows, view.Message(valuationReadTime(view, captured), view.Colors.SecondaryInk))
	}
	if p.openHistory != nil && row.AccountID != "" && result.Right != nil && result.Right.Current {
		rows = append(rows, view.Message(l.Text(i18n.WealthHistoryClosedLink), view.Colors.SecondaryInk))
	}
	if p.accounts.category.active {
		name := wealthAllocationName(l, p.accounts.category.key, p.accounts.category.currency)
		rows = append(rows, view.Message(fmt.Sprintf(l.Text(i18n.WealthAllocationFilter), name), view.Colors.Ink))
	}
	rows = append(rows, wealthDetailSide(view, result.Left, row.Left, 0, liability)...)
	if result.Right != nil {
		rows = append(rows, wealthDetailSide(view, *result.Right, row.Right, 1, liability)...)
		words := []string{l.Text(i18n.WealthAccountDelta), l.Text(i18n.WealthAccountIncomparable)}
		if result.Left.Currency == result.Right.Currency && wealthRowDelta(row) != nil {
			words[1] = wealthExactMoney(view, wealthRowDelta(row), result.Left.Currency, liability, true)
		}
		rows = append(rows, view.Message(strings.Join(words, " · "), view.Colors.Ink))
		rows = append(rows, wealthRowChanges(view, row)...)
	}
	var matching, others []layout.Widget
	for _, candidate := range result.Rows {
		if candidate.Key == row.ParentKey && row.ParentKey != "" {
			rows = append(rows, p.wealthDetailJump(view, sheet, candidate))
		}
		if candidate.ParentKey != row.Key {
			continue
		}
		link := p.wealthDetailJump(view, sheet, candidate)
		if p.accounts.category.matches(candidate) {
			matching = append(matching, link)
		} else {
			others = append(others, link)
		}
	}
	if len(matching) > 0 {
		rows = append(rows, view.Body(ui.TextLabel, l.Text(i18n.WealthRowComponents), view.Colors.Ink))
		rows = append(rows, matching...)
	}
	if len(others) > 0 {
		rows = append(rows, view.Body(ui.TextLabel, l.Text(i18n.WealthRowOtherComponents), view.Colors.Ink))
		rows = append(rows, others...)
	}
	return rows
}

func wealthDetailSide(view ui.View, endpoint application.HistoricalOverviewState, cell *application.HistoricalOverviewCell, side int, liability bool) []layout.Widget {
	l := view.Language
	rows := []layout.Widget{wealthEndpointCell(view, wealthEndpointLabel(l, view, endpoint, side), endpoint.Currency, cell, liability, true)}
	if cell != nil {
		if at, err := time.Parse(time.RFC3339Nano, cell.ValueSourceAt); err == nil {
			stamp := portfolioQuoteStamp(view, "", "", at)
			rows = append(rows, view.Message(l.Text(i18n.WealthRowSourceDate)+stamp, view.Colors.SecondaryInk))
		}
	}
	return rows
}

func wealthEndpointQuoteDetails(view ui.View, quote application.HistoricalOverviewEvidence) []string {
	l := view.Language
	point := domain.QuoteSeriesPoint{
		SourceKind:      domain.QuoteSourceKind(quote.Source),
		SourceKey:       quote.SourceKey,
		ObservationKind: quote.ObservationKind,
		PriceBasis:      quote.PriceBasis,
		Delayed:         quote.Freshness == string(domain.FreshnessDelayed),
	}
	var lines []string
	if value := wealthEndpointQuoteValue(view, quote); value != "" {
		label := i18n.WealthQuoteAppliedPrice
		if quote.BaseCurrency != "" && quote.QuoteCurrency != "" {
			label = i18n.WealthQuoteAppliedFX
		}
		lines = append(lines, l.Text(label)+" · "+value)
	}
	if point.SourceKind == domain.QuoteSourceManual {
		lines = append(lines, marketSourceLabel(l, point.SourceKind))
	} else {
		lines = append(lines, quoteHistoryEvidence(view, point)...)
	}
	switch {
	case quote.ClosedCarry:
		lines = append(lines, l.Text(i18n.WealthQuoteCarried))
	case quote.Freshness == string(domain.FreshnessStale):
		lines = append(lines, l.Text(i18n.WealthQuoteStale))
	}
	if day, ok := ui.ParseCivilDay(quote.MarketDate); ok {
		lines = append(lines, l.Text(i18n.WealthQuoteMarketDate)+" · "+ui.FormatDay(day, l, view.DateFormat))
	}
	if at, err := time.Parse(time.RFC3339Nano, quote.EffectiveAt); err == nil && quote.TimestampBasis != "date_label" {
		lines = append(lines, l.Text(i18n.WealthQuoteEffectiveAt)+" · "+holdingEvidenceTime(view, at, view.TimeLocation))
	}
	if quote.FetchedAt != nil {
		if at, err := time.Parse(time.RFC3339Nano, *quote.FetchedAt); err == nil {
			lines = append(lines, l.Text(i18n.WealthQuoteRetrievedAt)+" · "+holdingEvidenceTime(view, at, view.TimeLocation))
		}
	}
	switch quote.Operation {
	case "multiply":
		lines = append(lines, l.Text(i18n.WealthQuoteMultiply))
	case "divide":
		lines = append(lines, l.Text(i18n.WealthQuoteDivide))
	}
	return lines
}

func wealthRowChanges(view ui.View, row application.HistoricalOverviewRow) []layout.Widget {
	l := view.Language
	var rows []layout.Widget
	add := func(id i18n.MessageID) { rows = append(rows, view.Message(l.Text(id), view.Colors.SecondaryInk)) }
	if row.Left == nil || row.Right == nil {
		add(i18n.WealthAccountAbsent)
		return rows
	}
	if row.Left.Included != row.Right.Included {
		add(i18n.WealthHistoryScopeChanged)
	}
	if !wealthStatusComparable(row.Left.Status, row.Right.Status) {
		add(i18n.WealthChangeStatus)
	}
	if row.EstimatedBaseChange != nil &&
		(row.Left.DisplayValuation.Quality.Coverage == domain.CoverageProvisional || row.Right.DisplayValuation.Quality.Coverage == domain.CoverageProvisional) {
		add(i18n.WealthHistoryProvisional)
	}
	if row.Changed && row.BaseChange != nil && *row.BaseChange == "0" {
		add(i18n.WealthChangeOffsetting)
	}
	return rows
}

func (p *wealthHistoryPage) wealthDetailJump(view ui.View, sheet *wealthDetail, row application.HistoricalOverviewRow) layout.Widget {
	button := portfolioButton(sheet.links, row.Key)
	return func(gtx layout.Context) layout.Dimensions {
		if button.Clicked(gtx) {
			sheet.row = &row
			sheet.list.Position = layout.Position{}
			sheet.focus = true
			gtx.Execute(op.InvalidateCmd{})
		}
		return view.ButtonOf(button, ui.ButtonSpec{Title: wealthRowName(view.Language, row), Variant: ui.ButtonOutline, Size: ui.ButtonSmall})(gtx)
	}
}
