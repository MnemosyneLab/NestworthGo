package desktop

import (
	"fmt"
	"image"
	"image/color"
	"strings"

	"gioui.org/io/event"
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
	table    chartDataTable
}

type overviewBreakdowns struct {
	summaries [2]overviewSummaryChart
	stretch   [3]ui.RowHeights // composition, then each pair of dimensions
	links     map[accountListScope]*widget.Clickable
	open      func(navigationTarget, string, event.Tag)
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
		NotDrawn: l.Text(i18n.WealthAllocationNotDrawn), StackLegend: true, HideReadout: true}
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
	var totals [2]string
	for i, summary := range summaries {
		totals[i] = overviewBreakdownMoney(view, result.Currency, summary.total, summary.liability)
	}
	return (ui.Grid{MinColumn: 350, MaxColumns: 2, Gap: 16, Balanced: true, Rows: &b.stretch[0]}).Layout(gtx, view, len(summaries),
		func(gtx layout.Context, view ui.View, index int) layout.Dimensions {
			summary, chart := summaries[index], &b.summaries[0]
			if summary.liability {
				chart = &b.summaries[1]
			}
			donut, bars, useDonut := overviewSummarySpecs(view, summary, result.Currency)
			donut.CenterFit = totals[:len(summaries)]
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
	return (ui.Grid{MinColumn: 305, MaxColumns: 2, Gap: 16, Rows: &b.stretch[1+pair]}).Layout(gtx, view, 2,
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
			for i, item := range dimension.items {
				rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
					return b.dimensionRow(gtx, view, dimension.dimension, item, result.Currency, view.Colors.Category[(i+1)%len(view.Colors.Category)])
				}))
			}
			return overviewBreakdownCard(gtx, view, rows)
		})
}

func (b *overviewBreakdowns) dimensionRow(gtx layout.Context, view ui.View, dim accountDimension, item domain.BreakdownItem, currency domain.CurrencyCode, ink color.NRGBA) layout.Dimensions {
	scope := breakdownAccountScope(dim, item.Key)
	name := scope.name(view.Language, item.Label)
	money, share := overviewBreakdownMoney(view, currency, item.Amount, false), overviewBreakdownShare(view, item)
	label := strings.Join([]string{name, money, share}, " · ")
	target := navigationTarget{route: accountsRoute, accountList: scope}
	if dim == accountByInstitution && !scope.unassigned() {
		target.accountList = accountListScope{}
		target.institution = domain.InstitutionID(scope.key)
	}
	content := breakdownRow(view, name, money, share, item.ShareBPS, ink)
	if b.open == nil || !validAccountTarget(target) {
		return semanticRow(gtx, label, content)
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
	if button.Clicked(gtx) {
		b.open(target, name, button)
		gtx.Execute(op.InvalidateCmd{})
	}
	return ui.Bleed(gtx, ui.Space12, view.ButtonOf(button, ui.ButtonSpec{Name: label, Variant: ui.ButtonGhost, Fill: true, Content: content}))
}

func overviewBreakdownCard(gtx layout.Context, view ui.View, rows []ui.ColumnChild) layout.Dimensions {
	return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
		return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
	})
}

// breakdownRow is one entry of a dimension card as in Wails: a colored dot, the
// name, the share and the amount on one line, and a thin bar under it sized by
// the share. The bar repeats the printed share and carries no extra meaning.
func breakdownRow(view ui.View, name, amount, share string, shareBPS int, ink color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		colors := view.Colors
		line := func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					side := gtx.Dp(8)
					ui.FillRoundRect(gtx, image.Rect(0, 0, side, side), ui.RadiusFull, ink)
					return layout.Dimensions{Size: image.Pt(side, side)}
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return view.Line(gtx, ui.TextLabel, name, colors.Ink)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.Line(gtx, ui.TextCaption, share, colors.SecondaryInk)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.Line(gtx, ui.TextLabel, amount, colors.Ink)
				}),
			)
		}
		bar := func(gtx layout.Context) layout.Dimensions {
			height := gtx.Dp(6)
			width := gtx.Constraints.Max.X
			ui.FillRoundRect(gtx, image.Rect(0, 0, width, height), ui.RadiusFull, colors.HoverSurface)
			fill := width * min(max(shareBPS, 0), domain.TotalOwnershipBPS) / domain.TotalOwnershipBPS
			if fill > 0 {
				ui.FillRoundRect(gtx, image.Rect(0, 0, max(fill, height), height), ui.RadiusFull, ink)
			}
			return layout.Dimensions{Size: image.Pt(width, height)}
		}
		return (ui.Column{Gap: ui.Space6}).Layout(gtx, ui.Rigid(line), ui.Rigid(bar))
	}
}
