package desktop

import (
	"fmt"
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
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
)

type overviewSummaryChart struct {
	donut    charts.DonutChart
	bars     charts.BarChart
	selected charts.Selection
}

type overviewBreakdowns struct {
	summaries  [2]overviewSummaryChart
	jump       widget.Clickable
	focusFirst bool
	links      map[accountListScope]*widget.Clickable
	open       func(navigationTarget, string, event.Tag)
}

type overviewSummary struct {
	label     i18n.MessageID
	total     decimal.Decimal
	items     []domain.BreakdownItem
	quality   domain.ValuationQuality
	liability bool
}

func overviewSummaries(result domain.OverviewResult) []overviewSummary {
	var list []overviewSummary
	if len(result.AssetsByType) != 0 {
		list = append(list, overviewSummary{i18n.OverviewAssetsComposition, result.Assets, result.AssetsByType, result.DisplayValuation.Assets.Quality, false})
	}
	if len(result.LiabilitiesByType) != 0 && result.Liabilities.IsPositive() {
		list = append(list, overviewSummary{i18n.OverviewLiabilitiesComposition, result.Liabilities, result.LiabilitiesByType, result.DisplayValuation.Liabilities.Quality, true})
	}
	return list
}

func overviewBreakdownMoney(view ui.View, currency domain.CurrencyCode, amount decimal.Decimal, liability bool) string {
	return valuationMoney(view, domain.MoneyView{Currency: currency, Amount: amount.String()}, liability)
}

func overviewBreakdownShare(view ui.View, item domain.BreakdownItem) string {
	return portfolioDecimal(view, strings.TrimSuffix(format.ShareBPS(item.ShareBPS), "%")) + "%"
}

func overviewSummarySpecs(view ui.View, summary overviewSummary, currency domain.CurrencyCode) (charts.DonutSpec, charts.BarSpec, bool) {
	l := view.Language
	label := l.Text(summary.label)
	messages := charts.Messages{Missing: l.Text(i18n.OverviewNoValue), Selected: l.Text(i18n.ChartSelected), Empty: l.Text(i18n.OverviewAllocationUnavailable)}
	donut := charts.DonutSpec{Label: label, Messages: messages,
		CenterCaption: l.Text(i18n.OverviewKnownSubtotal), CenterValue: overviewBreakdownMoney(view, currency, summary.total, summary.liability),
		NotDrawn: l.Text(i18n.WealthAllocationNotDrawn)}
	bars := charts.BarSpec{Label: label, Messages: messages}
	useDonut := summary.total.IsPositive()
	for _, item := range summary.items {
		id, name := charts.ID(item.Key), wealthAllocationName(l, item.Key, false)
		amount, share := overviewBreakdownMoney(view, currency, item.Amount, summary.liability), overviewBreakdownShare(view, item)
		magnitude := charts.Num(item.Amount.InexactFloat64())
		useDonut = useDonut && magnitude.Valid() && !item.Amount.IsNegative() && item.ShareBPS >= 0 && item.ShareBPS <= domain.TotalOwnershipBPS
		donut.Slices = append(donut.Slices, charts.Slice{ID: id, Label: name, Value: magnitude, Text: amount, Share: share})
		signed := magnitude
		if summary.liability && item.Amount.IsPositive() {
			signed = charts.Num(item.Amount.Neg().InexactFloat64())
		}
		bars.Bars = append(bars.Bars, charts.Bar{ID: id, Label: name, Value: signed, Text: amount, Detail: []string{share}})
	}
	return donut, bars, useDonut
}

func (b *overviewBreakdowns) composition(gtx layout.Context, view ui.View, result domain.OverviewResult) layout.Dimensions {
	summaries := overviewSummaries(result)
	return (ui.Grid{MinColumn: 350, MaxColumns: 2, Gap: 10, Balanced: true}).Layout(gtx, view, len(summaries),
		func(gtx layout.Context, view ui.View, index int) layout.Dimensions {
			summary, chart := summaries[index], &b.summaries[0]
			if summary.liability {
				chart = &b.summaries[1]
			}
			donut, bars, useDonut := overviewSummarySpecs(view, summary, result.Currency)
			l := view.Language
			rows := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextHeading, l.Text(summary.label), view.Colors.Ink))}
			if summary.quality.UnpricedCount > 0 {
				rows = append(rows, ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.OverviewCompositionMissing), summary.quality.UnpricedCount), view.Colors.WarningInk)))
			}
			if !useDonut {
				rows = append(rows, ui.Rigid(view.Message(donut.CenterCaption+" · "+donut.CenterValue, view.Colors.SecondaryInk)))
			}
			rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				var dims layout.Dimensions
				var events []charts.Event
				if useDonut {
					donut.Selected = chart.selected
					dims, events = chart.donut.Layout(gtx, view, donut)
				} else {
					bars.Selected = chart.selected
					dims, events = chart.bars.Layout(gtx, view, bars)
				}
				if selected := charts.Apply(chart.selected, events); selected != chart.selected {
					chart.selected = selected
					gtx.Execute(op.InvalidateCmd{})
				}
				return dims
			}))
			return overviewBreakdownCard(gtx, view, rows)
		})
}

type overviewDimension struct {
	dimension accountDimension
	label     i18n.MessageID
	items     []domain.BreakdownItem
}

func overviewDimensions(result domain.OverviewResult) [4]overviewDimension {
	return [4]overviewDimension{
		{accountByInstitution, i18n.OverviewByInstitution, result.ByInstitution},
		{accountByType, i18n.OverviewByAccountType, result.ByAccountType},
		{accountByMember, i18n.OverviewByMember, result.ByMember},
		{accountByGroup, i18n.OverviewByGroup, result.ByGroup},
	}
}

func (b *overviewBreakdowns) dimensions(gtx layout.Context, view ui.View, result domain.OverviewResult, pair int) layout.Dimensions {
	dimensions := overviewDimensions(result)
	return (ui.Grid{MinColumn: 305, MaxColumns: 2, Gap: 10}).Layout(gtx, view, 2,
		func(gtx layout.Context, view ui.View, index int) layout.Dimensions {
			dimension := dimensions[pair*2+index]
			l := view.Language
			rows := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextHeading, l.Text(dimension.label), view.Colors.Ink))}
			if len(dimension.items) == 0 {
				rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.OverviewAllocationUnavailable), view.Colors.SecondaryInk)))
			}
			if dimension.dimension == accountByType {
				rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.OverviewAccountTypeHint), view.Colors.SecondaryInk)))
			}
			for _, item := range dimension.items {
				rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
					return b.dimensionRow(gtx, view, dimension.dimension, item, result.Currency)
				}))
			}
			return overviewBreakdownCard(gtx, view, rows)
		})
}

func (b *overviewBreakdowns) dimensionRow(gtx layout.Context, view ui.View, dim accountDimension, item domain.BreakdownItem, currency domain.CurrencyCode) layout.Dimensions {
	scope := breakdownAccountScope(dim, item.Key)
	name := scope.name(view.Language, item.Label)
	label := strings.Join([]string{name, overviewBreakdownMoney(view, currency, item.Amount, false), overviewBreakdownShare(view, item)}, " · ")
	target := navigationTarget{route: accountsRoute, accountList: scope}
	if dim == accountByInstitution && !scope.unassigned() {
		target.accountList = accountListScope{}
		target.institution = domain.InstitutionID(scope.key)
	}
	if b.open == nil || !validAccountTarget(target) {
		return view.Message(label, view.Colors.Ink)(gtx)
	}
	if b.links == nil {
		b.links = make(map[accountListScope]*widget.Clickable)
	}
	button := b.links[scope]
	if button == nil {
		button = new(widget.Clickable)
		b.links[scope] = button
	}
	view.Focus.Watch(gtx, button)
	if b.focusFirst {
		b.focusFirst = false
		gtx.Execute(key.FocusCmd{Tag: button})
	}
	if button.Clicked(gtx) {
		b.open(target, name, button)
		gtx.Execute(op.InvalidateCmd{})
	}
	return view.ButtonOf(button, ui.ButtonSpec{Title: label, Variant: ui.ButtonGhost, Fill: true})(gtx)
}

func overviewBreakdownCard(gtx layout.Context, view ui.View, rows []ui.ColumnChild) layout.Dimensions {
	return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return (ui.Column{Gap: 6}).Layout(gtx, rows...)
		})
	})
}
