package desktop

import (
	"fmt"
	"slices"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type wealthAllocationFilter struct {
	key      string
	currency bool
	role     domain.BalanceSheetRole
	active   bool
}

const wealthCategoryPrefix charts.ID = "category-"

func (f wealthAllocationFilter) matches(row application.HistoricalOverviewRow) bool {
	if !f.active {
		return true
	}
	if row.Role != string(f.role) {
		return false
	}
	for _, cell := range []*application.HistoricalOverviewCell{row.Left, row.Right} {
		if cell != nil && (f.currency && cell.Currency == f.key || !f.currency && cell.AssetClass == f.key) {
			return true
		}
	}
	return false
}

type wealthAllocation struct {
	currency, liabilities bool
	byClass, byCurrency   widget.Clickable
	assets, debts         widget.Clickable
	clear, back           widget.Clickable
	list                  widget.List
	donut                 charts.DonutChart
	bars                  charts.BarChart
	buttons               map[string]*widget.Clickable
	focus, restore        event.Tag
	scroll                int
	focusAccounts         bool
}

func (p *wealthHistoryPage) filterAllocation(view ui.View, key string, trigger event.Tag, comparison bool) {
	role := domain.RoleAsset
	if comparison && p.allocation.liabilities {
		role = domain.RoleLiability
	}
	p.accounts.category = wealthAllocationFilter{key: key, currency: p.allocation.currency, role: role, active: true}
	p.accounts.changesOnly = false
	p.accounts.list.Position = layout.Position{}
	p.allocation.focus = trigger
	p.allocation.scroll = view.PageScrollOffset()
	p.allocation.focusAccounts = true
	p.selectEndpointLens(wealthAccountsLens)
}

func (p *wealthHistoryPage) returnToAllocation(view ui.View) {
	if p.accounts.category.active {
		p.accounts.category = wealthAllocationFilter{}
		p.allocation.restore = p.allocation.focus
		p.allocation.focus = nil
		view.RestorePageScroll(p.allocation.scroll)
	}
	p.selectEndpointLens(wealthAllocationLens)
}

func (a *wealthAllocation) watchFocus(gtx layout.Context, view ui.View, control event.Tag) {
	view.Focus.Watch(gtx, control)
	if a.restore == control && gtx.Enabled() {
		a.restore = nil
		gtx.Execute(key.FocusCmd{Tag: control})
	}
}

var wealthAllocationBuckets = map[string]i18n.MessageID{
	domain.BucketCash: i18n.AccountCash, domain.BucketUnclassifiedInvestment: i18n.NavPortfolio,
	domain.BucketPension: i18n.AccountTypePension, domain.BucketInsurance: i18n.AccountTypeInsurance,
	domain.BucketProperty: i18n.AccountTypeProperty, domain.BucketVehicle: i18n.AccountTypeVehicle,
	domain.BucketCollectible: i18n.AccountTypeCollectible, domain.BucketReceivable: i18n.AccountTypeReceivable,
	domain.BucketCreditCard: i18n.AccountTypeCredit, domain.BucketLoan: i18n.AccountTypeLoan,
	domain.BucketOtherAsset: i18n.AssetOther, domain.BucketOtherLiability: i18n.AssetOther,
}

func wealthAllocationName(l i18n.Language, key string, currency bool) string {
	if key == "" {
		return l.Text(i18n.WealthAllocationUnclassified)
	}
	if currency {
		return key
	}
	if id := wealthAllocationBuckets[key]; id != i18n.NoMessage {
		return l.Text(id)
	}
	if id := marketInstrumentTypes[domain.InstrumentType(key)]; id != i18n.NoMessage {
		return l.Text(id)
	}
	return l.Text(i18n.OverviewEvidenceUnavailable)
}

func (p *wealthHistoryPage) allocationRows(gtx layout.Context, view ui.View, result application.HistoricalOverviewResult) []ui.ColumnChild {
	a, l := &p.allocation, view.Language
	for _, toggle := range []struct {
		button *widget.Clickable
		value  bool
		field  *bool
	}{
		{&a.byClass, false, &a.currency}, {&a.byCurrency, true, &a.currency},
		{&a.assets, false, &a.liabilities}, {&a.debts, true, &a.liabilities},
	} {
		if toggle.button.Clicked(gtx) {
			*toggle.field = toggle.value
			a.list.Position = layout.Position{}
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	controls := []layout.Widget{
		analysisToggle(view, &a.byClass, l.Text(i18n.WealthAllocationClasses), !a.currency),
		analysisToggle(view, &a.byCurrency, l.Text(i18n.WealthAllocationCurrencies), a.currency),
	}
	if result.Right != nil {
		controls = append(controls, analysisToggle(view, &a.assets, l.Text(i18n.Assets), !a.liabilities),
			analysisToggle(view, &a.debts, l.Text(i18n.Liabilities), a.liabilities))
	}
	rows := []ui.ColumnChild{ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		return (ui.Wrap{Gap: 8}).Layout(gtx, len(controls), func(gtx layout.Context, index int) layout.Dimensions {
			return controls[index](gtx)
		})
	})}
	if result.Right != nil {
		return append(rows, p.allocationCompareRows(view, result)...)
	}
	state := result.Left
	items := state.ByClass
	if a.currency {
		items = state.ByCurrency
	}
	rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.WealthAllocationBasis)+" · "+state.Currency, view.Colors.Ink)))
	if missing := state.DisplayValuation.Assets.Quality.UnpricedCount; missing > 0 {
		rows = append(rows, ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.WealthAllocationPartial), missing), view.Colors.SecondaryInk)))
	}
	if len(items) == 0 {
		return append(rows, ui.Rigid(view.Message(l.Text(i18n.WealthAllocationEmpty), view.Colors.SecondaryInk)))
	}
	donut, bar, useDonut := wealthAllocationCharts(view, state, items, a.currency)
	shown, available := false, false
	for _, item := range bar.Bars {
		available = available || item.Value.Valid()
		shown = shown || item.Value.Valid() && item.Value.Number != 0
	}
	if shown {
		rows = append(rows, ui.Rigid(p.allocationChart(view, donut, bar, useDonut)))
	} else if available {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.WealthAllocationZero), view.Colors.SecondaryInk)))
	} else {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.OverviewNoValue), view.Colors.SecondaryInk)))
	}
	rows = append(rows, ui.Fill(func(gtx layout.Context) layout.Dimensions {
		return analysisResultList(gtx, view, &a.list, l.Text(i18n.WealthAllocationTable), len(items), func(gtx layout.Context, index int) layout.Dimensions {
			item := items[index]
			name := wealthAllocationName(l, item.Key, a.currency)
			label := strings.Join([]string{name, wealthExactMoney(view, &item.Amount, state.Currency, false, false), wealthAllocationShare(view, &item.Share)}, " · ")
			return p.allocationRowButton(gtx, view, item.Key, label, false)
		})
	}))
	return rows
}

func wealthAllocationCharts(view ui.View, state application.HistoricalOverviewState, items []application.HistoricalOverviewAllocation, currency bool) (charts.DonutSpec, charts.BarSpec, bool) {
	l := view.Language
	label, summary := l.Text(i18n.WealthAllocationTab), l.Text(i18n.WealthAllocationChartHelp)
	messages := charts.Messages{Empty: l.Text(i18n.WealthAllocationEmpty), Missing: l.Text(i18n.OverviewNoValue), Selected: l.Text(i18n.ChartSelected)}
	donut := charts.DonutSpec{Label: label, Summary: summary, Messages: messages,
		CenterCaption: l.Text(i18n.WealthAllocationBasis), CenterValue: wealthExactMoney(view, &state.KnownAssets, state.Currency, false, false),
		NotDrawn: l.Text(i18n.WealthAllocationNotDrawn), ZeroTotal: l.Text(i18n.WealthAllocationEmpty)}
	bars := charts.BarSpec{Label: label, Summary: summary, Messages: messages}
	known, err := decimal.NewFromString(state.KnownAssets)
	useDonut := err == nil && known.IsPositive()
	for _, item := range items {
		name := wealthAllocationName(l, item.Key, currency)
		amount, amountErr := decimal.NewFromString(item.Amount)
		share, shareErr := decimal.NewFromString(item.Share)
		value := charts.NoValue
		if amountErr == nil {
			value = charts.Num(amount.InexactFloat64())
		}
		useDonut = useDonut && value.Valid() && !amount.IsNegative() && shareErr == nil && !share.IsNegative() && !share.GreaterThan(decimal.NewFromInt(100))
		text := wealthExactMoney(view, &item.Amount, state.Currency, false, false)
		percent := wealthAllocationShare(view, &item.Share)
		id := wealthCategoryPrefix + charts.ID(item.Key)
		donut.Slices = append(donut.Slices, charts.Slice{ID: id, Label: name, Value: value, Text: text, Share: percent})
		bars.Bars = append(bars.Bars, charts.Bar{ID: id, Label: name, Value: value, Text: text, Detail: []string{percent}})
	}
	return donut, bars, useDonut
}

func (p *wealthHistoryPage) allocationChart(view ui.View, donut charts.DonutSpec, bar charts.BarSpec, useDonut bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
			a := &p.allocation
			var dims layout.Dimensions
			var events []charts.Event
			var trigger event.Tag
			if useDonut {
				trigger = &a.donut
				a.watchFocus(gtx, view, trigger)
				dims, events = a.donut.Layout(gtx, view, donut)
			} else {
				trigger = &a.bars
				a.watchFocus(gtx, view, trigger)
				dims, events = a.bars.Layout(gtx, view, bar)
			}
			if selected := charts.Apply(charts.Selection{}, events); !selected.IsZero() {
				p.filterAllocation(view, strings.TrimPrefix(string(selected.Item), string(wealthCategoryPrefix)), trigger, false)
				gtx.Execute(op.InvalidateCmd{})
			}
			return dims
		})
	}
}

func (p *wealthHistoryPage) allocationRowButton(gtx layout.Context, view ui.View, key, label string, compare bool) layout.Dimensions {
	a := &p.allocation
	if a.buttons == nil {
		a.buttons = make(map[string]*widget.Clickable)
	}
	button := portfolioButton(a.buttons, key)
	a.watchFocus(gtx, view, button)
	if button.Clicked(gtx) {
		p.filterAllocation(view, key, button, compare)
		gtx.Execute(op.InvalidateCmd{})
	}
	return view.ButtonOf(button, ui.ButtonSpec{Title: label, Variant: ui.ButtonGhost, Fill: true})(gtx)
}

func wealthAllocationShare(view ui.View, share *string) string {
	if share == nil || *share == "" {
		return view.Language.Text(i18n.OverviewNoValue)
	}
	return portfolioDecimal(view, *share) + "%"
}

func (p *wealthHistoryPage) allocationCompareRows(view ui.View, result application.HistoricalOverviewResult) []ui.ColumnChild {
	a, l := &p.allocation, view.Language
	role := domain.RoleAsset
	if a.liabilities {
		role = domain.RoleLiability
	}
	var items []application.HistoricalCategoryComparison
	for _, item := range result.Categories {
		if item.Role == string(role) && (a.currency && item.Dimension == "byCurrency" || !a.currency && item.Dimension == "byClass") {
			items = append(items, item)
		}
	}
	slices.SortStableFunc(items, func(a, b application.HistoricalCategoryComparison) int {
		switch {
		case a.Right.Amount == nil && b.Right.Amount != nil:
			return 1
		case a.Right.Amount != nil && b.Right.Amount == nil:
			return -1
		case a.Right.Amount != nil && b.Right.Amount != nil:
			left, errA := decimal.NewFromString(*a.Right.Amount)
			right, errB := decimal.NewFromString(*b.Right.Amount)
			if order := right.Cmp(left); errA == nil && errB == nil && order != 0 {
				return order
			}
		}
		return strings.Compare(a.Key, b.Key)
	})
	if len(items) == 0 {
		return []ui.ColumnChild{ui.Rigid(view.Message(l.Text(i18n.WealthAllocationNoCategories), view.Colors.SecondaryInk))}
	}
	rows := make(map[string]application.HistoricalOverviewRow, len(result.Rows))
	for _, row := range result.Rows {
		rows[row.Key] = row
	}
	leftCurrency, rightCurrency := result.Left.Currency, result.Right.Currency
	var children []ui.ColumnChild
	if leftCurrency != rightCurrency {
		children = append(children, ui.Rigid(view.Message(l.Text(i18n.WealthAllocationCurrencyChanged), view.Colors.SecondaryInk)))
	}
	return append(children, ui.Fill(func(gtx layout.Context) layout.Dimensions {
		return analysisResultList(gtx, view, &a.list, l.Text(i18n.WealthAllocationCompare), len(items), func(gtx layout.Context, index int) layout.Dimensions {
			item := items[index]
			name := wealthAllocationName(l, item.Key, a.currency)
			return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return (ui.Column{Gap: 6}).Layout(gtx,
						ui.Rigid(func(gtx layout.Context) layout.Dimensions {
							return p.allocationRowButton(gtx, view, item.Key, name, true)
						}),
						ui.Rigid(func(gtx layout.Context) layout.Dimensions {
							return (ui.Grid{MinColumn: 170, MaxColumns: 2, Gap: 6}).Layout(gtx, view, 2,
								func(gtx layout.Context, view ui.View, index int) layout.Dimensions {
									if index == 0 {
										return wealthCategoryCell(view, i18n.WealthAccountSideA, item.Left, leftCurrency, a.liabilities, rows)(gtx)
									}
									return wealthCategoryCell(view, i18n.WealthAccountSideB, item.Right, rightCurrency, a.liabilities, rows)(gtx)
								})
						}),
						ui.Rigid(wealthCategoryDelta(view, item, leftCurrency, leftCurrency == rightCurrency, a.liabilities)),
					)
				})
			})
		})
	}))
}

func wealthCategoryCell(view ui.View, side i18n.MessageID, cell application.HistoricalCategoryCell, currency string, liability bool,
	rows map[string]application.HistoricalOverviewRow) layout.Widget {
	l := view.Language
	value := wealthExactMoney(view, cell.Amount, currency, liability, false)
	switch cell.Status {
	case "absent":
		value = l.Text(i18n.WealthAccountAbsent)
	case "excluded":
		value = l.Text(i18n.WealthAccountExcluded)
	case "partial":
		value = l.Text(i18n.WealthAllocationMissing)
	}
	children := []ui.ColumnChild{ui.Rigid(view.Message(l.Text(side)+" · "+value, view.Colors.Ink))}
	if cell.Status == "partial" {
		known := l.Text(i18n.OverviewKnownSubtotal) + " · " + wealthExactMoney(view, cell.KnownAmount, currency, liability, false)
		children = append(children, ui.Rigid(view.Message(known, view.Colors.SecondaryInk)))
		for _, key := range cell.MissingRowKeys {
			row := rows[key]
			name := row.Name
			if parent := rows[row.ParentKey].Name; parent != "" && parent != name {
				name = parent + " · " + name
			}
			if name != "" {
				children = append(children, ui.Rigid(view.Message(name, view.Colors.SecondaryInk)))
			}
		}
	}
	if cell.Share != nil {
		share := wealthAllocationShare(view, cell.Share)
		if cell.ShareBasis == "known_subtotal" {
			share = fmt.Sprintf(l.Text(i18n.WealthAllocationKnownShare), share)
		}
		children = append(children, ui.Rigid(view.Message(share, view.Colors.SecondaryInk)))
	}
	return func(gtx layout.Context) layout.Dimensions { return (ui.Column{Gap: 4}).Layout(gtx, children...) }
}

func wealthCategoryDelta(view ui.View, item application.HistoricalCategoryComparison, currency string, comparable, liability bool) layout.Widget {
	l := view.Language
	amount, share := l.Text(i18n.WealthAccountIncomparable), l.Text(i18n.WealthAccountIncomparable)
	if comparable && item.AmountChange != nil {
		amount = wealthExactMoney(view, item.AmountChange, currency, liability, true)
	}
	if comparable && item.ShareChangePp != nil {
		value := portfolioDecimal(view, *item.ShareChangePp)
		if parsed, err := decimal.NewFromString(*item.ShareChangePp); err == nil && parsed.IsPositive() {
			value = "+" + value
		}
		share = fmt.Sprintf(l.Text(i18n.WealthAllocationPoints), value)
	}
	children := []ui.ColumnChild{
		ui.Rigid(view.Message(l.Text(i18n.WealthAccountDelta)+" · "+amount, view.Colors.Ink)),
		ui.Rigid(view.Message(l.Text(i18n.WealthAllocationShareChange)+" · "+share, view.Colors.SecondaryInk)),
	}
	seen := map[i18n.MessageID]bool{}
	for _, codes := range [][]string{item.AmountReasons, item.ShareReasons} {
		for _, code := range codes {
			if id := wealthCategoryReason(code); id != i18n.NoMessage && !seen[id] {
				seen[id] = true
				children = append(children, ui.Rigid(view.Message(l.Text(id), view.Colors.SecondaryInk)))
			}
		}
	}
	return func(gtx layout.Context) layout.Dimensions { return (ui.Column{Gap: 4}).Layout(gtx, children...) }
}

func wealthCategoryReason(code string) i18n.MessageID {
	switch code {
	case "existence_changed":
		return i18n.WealthAllocationExistenceChanged
	case "scope_changed":
		return i18n.WealthAllocationScopeChanged
	case "membership_changed":
		return i18n.WealthAllocationMembershipChanged
	case "share_denominator_unavailable":
		return i18n.WealthAllocationShareUnavailable
	default:
		return i18n.NoMessage
	}
}
