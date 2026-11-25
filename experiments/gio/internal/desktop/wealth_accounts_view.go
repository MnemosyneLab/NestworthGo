package desktop

import (
	"fmt"
	"image"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *wealthHistoryPage) layoutWealthTabs(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	buttons := []layout.Widget{
		view.ButtonOf(&p.accountsTab, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall,
			Title: l.Text(i18n.NavAccounts), Name: l.Text(i18n.WealthAccountTabName), Toggle: true, Selected: p.showAccounts}),
		analysisToggle(view, &p.trendTab, l.Text(i18n.WealthTrendTab), !p.showAccounts),
	}
	return (ui.Wrap{Gap: 8}).Layout(gtx, len(buttons), func(gtx layout.Context, index int) layout.Dimensions {
		return buttons[index](gtx)
	})
}

func (p *wealthHistoryPage) layoutWealthAccounts(gtx layout.Context, view ui.View, now time.Time) layout.Dimensions {
	p.updateAccounts()
	a, l := &p.accounts, view.Language
	labels := [...]i18n.MessageID{i18n.WealthAccountSingle, i18n.WealthAccountCompareDate, i18n.WealthAccountCompareCurrent}
	for index := range a.modes {
		if a.modes[index].Clicked(gtx) {
			a.draft.mode = wealthAccountMode(index)
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	if a.draft.mode == wealthCompareDate && a.swap.Clicked(gtx) {
		a.draft.from, a.draft.to = a.draft.to, a.draft.from
		gtx.Execute(op.InvalidateCmd{})
	}
	valid := p.validAccountDates(a.draft, now)
	if a.reload.Clicked(gtx) && valid && (a.request == nil || a.draft != a.requested) {
		p.readAccounts(a.draft)
		gtx.Execute(op.InvalidateCmd{})
	}
	width, gap := gtx.Constraints.Max.X, gtx.Dp(10)
	modes := (ui.Wrap{Gap: 8}).Layout(gtx, len(labels), func(gtx layout.Context, index int) layout.Dimensions {
		return analysisToggle(view, &a.modes[index], l.Text(labels[index]), a.draft.mode == wealthAccountMode(index))(gtx)
	})
	offset := modes.Size.Y + gap
	dates := ui.Place(gtx, view, image.Pt(0, offset), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
		last := p.origin.lastClosed(now).Format(time.DateOnly)
		options := ui.DateOptions{Today: last, Min: p.origin.first, Max: last, Format: view.DateFormat, WeekStart: time.Monday}
		if view.WeekStart == "sunday" {
			options.WeekStart = time.Sunday
		}
		fields := []analysisFilterField{analysisDate(&a.earlier, i18n.WealthAccountDateA, options, a.draft.from, func(value string) {
			a.draft.from = value
			gtx.Execute(op.InvalidateCmd{})
		})}
		if a.draft.mode == wealthCompareDate {
			fields = append(fields, analysisDate(&a.later, i18n.WealthAccountDateB, options, a.draft.to, func(value string) {
				a.draft.to = value
				gtx.Execute(op.InvalidateCmd{})
			}))
		}
		return (ui.Grid{MinColumn: 200, MaxColumns: 2, Gap: 10}).Layout(gtx, view, len(fields),
			func(gtx layout.Context, view ui.View, index int) layout.Dimensions { return fields[index](gtx, view) })
	})
	offset += dates.Size.Y + gap
	body := ui.Place(gtx, view, image.Pt(0, offset), width, func(gtx layout.Context, view ui.View) layout.Dimensions {
		label := i18n.WealthAccountCapture
		switch {
		case a.request != nil:
			label = i18n.WealthAccountLoading
		case a.err != nil && a.draft == a.requested:
			label = i18n.HistoryRetry
		case a.current != nil && a.draft == a.captured:
			label = i18n.HistoryRefresh
		}
		buttons := []layout.Widget{returnMonthButton(view, &a.reload, label, nil, valid && (a.request == nil || a.draft != a.requested))}
		if a.draft.mode == wealthCompareDate {
			buttons = append(buttons, returnMonthButton(view, &a.swap, i18n.WealthAccountSwap, nil, a.draft.from != a.draft.to))
		}
		rows := []ui.ColumnChild{ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return (ui.Wrap{Gap: 8}).Layout(gtx, len(buttons), func(gtx layout.Context, index int) layout.Dimensions {
				return buttons[index](gtx)
			})
		})}
		if a.err != nil {
			rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.WealthAccountFailed), view.Colors.ErrorInk)))
			if a.current != nil {
				rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.WealthHistoryRetained), view.Colors.SecondaryInk)))
			}
		}
		if result := a.current; result != nil {
			rows = append(rows, wealthAccountSummary(view, *result)...)
		}
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions { return p.layoutWealthTabs(gtx, view) }))
		if result := a.current; result != nil {
			rows = append(rows, a.accountRows(gtx, view, *result)...)
		}
		return (ui.Column{Gap: 10, MinFill: 310}).Layout(gtx, rows...)
	})
	return layout.Dimensions{Size: image.Pt(width, offset+body.Size.Y)}
}

func wealthAccountSummary(view ui.View, result application.HistoricalOverviewResult) []ui.ColumnChild {
	l := view.Language
	rows := []ui.ColumnChild{}
	if captured, err := time.Parse(time.RFC3339Nano, result.CapturedAt); err == nil {
		rows = append(rows, ui.Rigid(view.Message(valuationReadTime(view, captured), view.Colors.SecondaryInk)))
	}
	return append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		endpoints := []application.HistoricalOverviewState{result.Left}
		if result.Right != nil {
			endpoints = append(endpoints, *result.Right)
		}
		return (ui.Grid{MinColumn: 205, MaxColumns: 2, Gap: 10}).Layout(gtx, view, len(endpoints),
			func(gtx layout.Context, view ui.View, index int) layout.Dimensions {
				endpoint := endpoints[index]
				label := wealthEndpointLabel(l, view, endpoint, index)
				return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						v := endpoint.DisplayValuation
						return (ui.Column{Gap: 5}).Layout(gtx,
							ui.Rigid(view.Body(ui.TextLabel, label+" · "+endpoint.Currency, view.Colors.Ink)),
							ui.Rigid(view.Message(wealthAmount(view, i18n.NetWorth, v.NetWorth), view.Colors.Ink)),
							ui.Rigid(view.Message(wealthAmount(view, i18n.Assets, v.Assets), view.Colors.Ink)),
							ui.Rigid(view.Message(wealthAmount(view, i18n.Liabilities, v.Liabilities), view.Colors.Ink)))
					})
				})
			})
	}))
}

func wealthEndpointLabel(l i18n.Language, view ui.View, endpoint application.HistoricalOverviewState, index int) string {
	date := wealthDay(view, endpoint.Date)
	if endpoint.Current {
		date = l.Text(i18n.WealthAccountCurrent)
	}
	if index == 0 {
		return l.Text(i18n.WealthAccountSideA) + " · " + date
	}
	return l.Text(i18n.WealthAccountSideB) + " · " + date
}

func (a *wealthAccountCapture) accountRows(gtx layout.Context, view ui.View, result application.HistoricalOverviewResult) []ui.ColumnChild {
	l := view.Language
	var rows []ui.ColumnChild
	if result.Right != nil {
		if a.changes.Clicked(gtx) {
			a.changesOnly = !a.changesOnly
			a.list.Position = layout.Position{}
			gtx.Execute(op.InvalidateCmd{})
		}
		change := l.Text(i18n.OverviewNoValue)
		if result.Change != nil && result.Left.Currency == result.Right.Currency {
			change = wealthExactMoney(view, result.Change.NetWorth, result.Left.Currency, false, true)
		}
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.WealthAccountNetChange)+" · "+change, view.Colors.Ink)),
			ui.Rigid(analysisToggle(view, &a.changes, l.Text(i18n.WealthAccountChanges), a.changesOnly)))
	}
	if a.expanded == nil {
		a.expanded = make(map[string]bool)
		a.toggles = make(map[string]*widget.Clickable)
	}
	var visible []application.HistoricalOverviewRow
	accounts := make(map[string]bool)
	for _, row := range result.Rows {
		if row.Kind == "account" && (result.Right == nil || !a.changesOnly || row.Changed) {
			accounts[row.Key] = true
		}
	}
	for _, row := range result.Rows {
		if accounts[row.Key] || accounts[row.ParentKey] && a.expanded[row.ParentKey] {
			visible = append(visible, row)
		}
	}
	if len(visible) == 0 {
		return append(rows, ui.Rigid(view.Message(l.Text(i18n.WealthAccountEmpty), view.Colors.SecondaryInk)))
	}
	return append(rows, ui.Fill(func(gtx layout.Context) layout.Dimensions {
		return analysisResultList(gtx, view, &a.list, l.Text(i18n.WealthAccountTabName), len(visible), func(gtx layout.Context, index int) layout.Dimensions {
			return a.accountRow(gtx, view, result, visible[index])
		})
	}))
}

func (a *wealthAccountCapture) accountRow(gtx layout.Context, view ui.View, result application.HistoricalOverviewResult, row application.HistoricalOverviewRow) layout.Dimensions {
	l := view.Language
	name := row.Name
	if row.Kind == "balance" {
		name = l.Text(i18n.WealthAccountBalance)
	}
	if row.Kind == "cash" {
		name = l.Text(i18n.AccountCash) + " · " + name
	}
	if name == "" || name == row.HoldingID || name == row.AccountID {
		name = l.Text(i18n.OverviewEvidenceUnavailable)
	}
	controls := []ui.ColumnChild{}
	if row.Kind == "account" {
		button := portfolioButton(a.toggles, row.Key)
		if button.Clicked(gtx) {
			a.expanded[row.Key] = !a.expanded[row.Key]
			gtx.Execute(op.InvalidateCmd{})
		}
		controls = append(controls, ui.Rigid(view.ButtonOf(button, ui.ButtonSpec{
			Variant: ui.ButtonGhost, Size: ui.ButtonSmall, Title: name,
			Description: l.Text(i18n.WealthAccountComponents), Toggle: true, Selected: a.expanded[row.Key],
		})))
	} else {
		controls = append(controls, ui.Rigid(view.Body(ui.TextLabel, name, view.Colors.Ink)))
	}
	liability := row.Role == string(domain.RoleLiability)
	sides := []layout.Widget{wealthEndpointCell(view, l.Text(i18n.WealthAccountSideA), result.Left.Currency, row.Left, liability)}
	if result.Right != nil {
		sides = append(sides, wealthEndpointCell(view, l.Text(i18n.WealthAccountSideB), result.Right.Currency, row.Right, liability))
		delta := l.Text(i18n.WealthAccountIncomparable)
		if row.Left != nil && row.Right != nil && result.Left.Currency == result.Right.Currency {
			delta = wealthExactMoney(view, row.BaseChange, result.Left.Currency, liability, true)
		}
		sides = append(sides, view.Message(l.Text(i18n.WealthAccountDelta)+" · "+delta, view.Colors.Ink))
	}
	controls = append(controls, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		return (ui.Grid{MinColumn: 165, MaxColumns: 3, Gap: 8}).Layout(gtx, view, len(sides),
			func(gtx layout.Context, _ ui.View, index int) layout.Dimensions { return sides[index](gtx) })
	}))
	inset := layout.UniformInset(10)
	if row.ParentKey != "" {
		inset.Left = 24
	}
	return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
		return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return (ui.Column{Gap: 6}).Layout(gtx, controls...)
		})
	})
}

func wealthEndpointCell(view ui.View, side, currency string, cell *application.HistoricalOverviewCell, liability bool) layout.Widget {
	l := view.Language
	if cell == nil {
		return view.Message(side+" · "+l.Text(i18n.WealthAccountAbsent), view.Colors.SecondaryInk)
	}
	base := wealthExactMoney(view, cell.BaseAmount, currency, liability, false)
	switch {
	case cell.DisplayValuation.Total != nil:
		base = valuationMoney(view, *cell.DisplayValuation.Total, liability)
	case cell.DisplayValuation.KnownSubtotal != nil:
		base = l.Text(i18n.OverviewKnownSubtotal) + " · " + valuationMoney(view, *cell.DisplayValuation.KnownSubtotal, liability)
	}
	rows := []ui.ColumnChild{ui.Rigid(view.Message(side+" · "+base, view.Colors.Ink))}
	if cell.NativeAmount != nil && cell.Currency != currency {
		native := wealthExactMoney(view, cell.NativeAmount, cell.Currency, liability, false)
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.AnalysisNative)+" · "+native, view.Colors.SecondaryInk)))
	}
	if cell.Quantity != nil {
		rows = append(rows, ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.AccountQuantity), portfolioDecimal(view, *cell.Quantity)), view.Colors.SecondaryInk)))
	}
	var statuses []string
	if !cell.Included {
		statuses = append(statuses, l.Text(i18n.WealthAccountExcluded))
	}
	if !cell.Complete {
		statuses = append(statuses, l.Text(i18n.OverviewIncomplete))
	}
	if cell.Status == "archived" {
		statuses = append(statuses, l.Text(i18n.DirectoryArchived))
	}
	if len(statuses) > 0 {
		rows = append(rows, ui.Rigid(view.Message(strings.Join(statuses, " · "), view.Colors.SecondaryInk)))
	}
	for _, missing := range cell.MissingDetails {
		if message, known := overviewMissing[domain.MissingInputKind(missing.Kind)]; known {
			parts := []string{l.Text(message)}
			if missing.BaseCurrency != "" && missing.QuoteCurrency != "" {
				parts = append(parts, missing.BaseCurrency+" / "+missing.QuoteCurrency)
			}
			if missing.Source != "" {
				parts = append(parts, marketSourceLabel(l, domain.QuoteSourceKind(missing.Source)))
			}
			rows = append(rows, ui.Rigid(view.Message(strings.Join(parts, " · "), view.Colors.SecondaryInk)))
		}
	}
	for _, evidence := range []*application.HistoricalOverviewEvidence{cell.Price, cell.FX} {
		if evidence != nil {
			rows = append(rows, ui.Rigid(view.Message(wealthEndpointQuote(view, *evidence), view.Colors.SecondaryInk)))
		}
	}
	return func(gtx layout.Context) layout.Dimensions { return (ui.Column{Gap: 4}).Layout(gtx, rows...) }
}

func wealthEndpointQuote(view ui.View, quote application.HistoricalOverviewEvidence) string {
	at, _ := time.Parse(time.RFC3339Nano, quote.EffectiveAt)
	source := domain.QuoteSourceKind(quote.Source)
	key := quote.SourceKey
	if source == domain.QuoteSourceAgent || key == quote.Source {
		key = ""
	}
	label := marketSourceLabel(view.Language, source) + portfolioQuoteStamp(view, key, quote.MarketDate, at)
	if quote.Value != nil {
		value := portfolioDecimal(view, *quote.Value)
		if quote.BaseCurrency != "" && quote.QuoteCurrency != "" {
			value = fmt.Sprintf(view.Language.Text(i18n.WealthAccountRate), quote.BaseCurrency, value, quote.QuoteCurrency)
		} else if quote.Currency != "" {
			value = quote.Currency + " " + value
		}
		label = value + " · " + label
	}
	return label
}

func wealthExactMoney(view ui.View, value *string, currency string, liability, delta bool) string {
	if value == nil {
		return view.Language.Text(i18n.OverviewNoValue)
	}
	amount := *value
	if delta {
		parsed, err := decimal.NewFromString(amount)
		if liability && err == nil {
			parsed = parsed.Neg()
			amount = parsed.String()
		}
		formatted := valuationMoney(view, domain.MoneyView{Currency: domain.CurrencyCode(currency), Amount: amount}, false)
		if err == nil && parsed.IsPositive() {
			return "+" + formatted
		}
		return formatted
	}
	return valuationMoney(view, domain.MoneyView{Currency: domain.CurrencyCode(currency), Amount: amount}, liability)
}
