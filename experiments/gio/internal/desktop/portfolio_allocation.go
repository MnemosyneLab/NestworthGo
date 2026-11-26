package desktop

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"github.com/shopspring/decimal"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
)

type portfolioAllocationRead struct {
	firstDay  string
	location  *time.Location
	originErr error
	accounts  []domain.AccountValuation
}

func readPortfolioAllocation(ctx context.Context, service portfolioReader) (out portfolioRead, err error) {
	err = service.WithRead(ctx, func(ctx context.Context) error {
		summary, err := service.Portfolio(ctx, domain.AccountFilter{})
		if err != nil {
			return err
		}
		out.summary = summary
		allocation := &portfolioAllocationRead{accounts: sortedPortfolioAccounts(summary.Accounts)}
		if len(allocation.accounts) != 0 {
			origin, err := service.HistoryOrigin(ctx)
			allocation.originErr = err
			if err == nil && origin != nil {
				allocation.location, allocation.originErr = time.LoadLocation(origin.Timezone)
				if allocation.originErr == nil {
					allocation.firstDay = origin.StartedAt.In(allocation.location).Format(time.DateOnly)
				}
			}
		}
		out.allocation = allocation
		return nil
	})
	return out, err
}

func sortedPortfolioAccounts(accounts []domain.AccountValuation) []domain.AccountValuation {
	type rankedAccount struct {
		index  int
		amount decimal.Decimal
		valued bool
	}
	order := make([]rankedAccount, 0, len(accounts))
	for i, valuation := range accounts {
		row := rankedAccount{index: i}
		if valuation.BaseValue != nil {
			amount, err := decimal.NewFromString(valuation.BaseValue.Amount)
			row.amount, row.valued = amount, err == nil
		}
		order = append(order, row)
	}
	slices.SortStableFunc(order, func(a, b rankedAccount) int {
		if a.valued != b.valued {
			if a.valued {
				return -1
			}
			return 1
		}
		if a.valued {
			if cmp := b.amount.Cmp(a.amount); cmp != 0 {
				return cmp
			}
		}
		return strings.Compare(accounts[a.index].Account.Name, accounts[b.index].Account.Name)
	})
	ranked := make([]domain.AccountValuation, 0, len(order))
	for _, row := range order {
		ranked = append(ranked, accounts[row.index])
	}
	return ranked
}

func (p *portfolioPage) allocationRows(view ui.View, data domain.PortfolioValuation) []layout.Widget {
	l := view.Language
	accounts := p.current.allocation.accounts
	if len(accounts) == 0 {
		return []layout.Widget{emptyState(view, l.Text(i18n.PortfolioAllocationEmpty))}
	}
	rows := make([]layout.Widget, 0, len(accounts)+3)
	if len(data.ByInstrumentType) > 0 {
		rows = append(rows, p.portfolioAllocationCard(view, data))
	}
	rows = append(rows, p.portfolioTrendCard(view), view.Body(ui.TextHeading, l.Text(i18n.PortfolioAllocationAccounts), view.Colors.Ink))
	for _, account := range accounts {
		rows = append(rows, p.portfolioAllocationAccount(view, account))
	}
	return rows
}

func portfolioAllocationSpecs(view ui.View, data domain.PortfolioValuation) (charts.DonutSpec, charts.BarSpec, bool) {
	l := view.Language
	label := l.Text(i18n.WealthAllocationTab)
	messages := charts.Messages{Missing: l.Text(i18n.OverviewNoValue), Selected: l.Text(i18n.ChartSelected), Empty: l.Text(i18n.OverviewAllocationUnavailable)}
	donut := charts.DonutSpec{Label: label, Messages: messages,
		CenterCaption: l.Text(i18n.PortfolioValuedHoldings), CenterValue: l.Text(i18n.OverviewNoValue),
		NotDrawn: l.Text(i18n.WealthAllocationNotDrawn), ZeroTotal: l.Text(i18n.WealthAllocationZero)}
	center := data.ValuedSubtotal
	useDonut := false
	if center != nil {
		donut.CenterValue = valuationMoney(view, *center, false)
		amount, err := decimal.NewFromString(center.Amount)
		useDonut = err == nil && amount.IsPositive()
	}
	bars := charts.BarSpec{Label: label, Messages: messages}
	for _, row := range data.ByInstrumentType {
		id, name := charts.ID(row.Key), wealthAllocationName(l, row.Key, false)
		amount, err := decimal.NewFromString(row.Amount.Amount)
		value := charts.NoValue
		if err == nil {
			value = charts.Num(amount.InexactFloat64())
		}
		useDonut = useDonut && value.Valid() && !amount.IsNegative() && row.ShareBPS >= 0 && row.ShareBPS <= domain.TotalOwnershipBPS
		text := valuationMoney(view, row.Amount, false)
		share := portfolioDecimal(view, strings.TrimSuffix(format.ShareBPS(row.ShareBPS), "%")) + "%"
		donut.Slices = append(donut.Slices, charts.Slice{ID: id, Label: name, Value: value, Text: text, Share: share})
		bars.Bars = append(bars.Bars, charts.Bar{ID: id, Label: name, Value: value, Text: text, Detail: []string{share}})
	}
	return donut, bars, useDonut
}

func (p *portfolioPage) portfolioAllocationCard(view ui.View, data domain.PortfolioValuation) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		chart, l := &p.allocation.composition, view.Language
		donut, bars, useDonut := portfolioAllocationSpecs(view, data)
		rows := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextHeading, donut.Label, view.Colors.Ink))}
		if !data.Complete {
			rows = append(rows, ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.OverviewCompositionMissing), len(data.MissingInputs)), view.Colors.WarningInk)))
		}
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			var dims layout.Dimensions
			var events []charts.Event
			if useDonut {
				donut.Selected = chart.selected
				view.Focus.Watch(gtx, &chart.donut)
				dims, events = chart.donut.Layout(gtx, view, donut)
			} else {
				bars.Selected = chart.selected
				view.Focus.Watch(gtx, &chart.bars)
				dims, events = chart.bars.Layout(gtx, view, bars)
			}
			if selected := charts.Apply(chart.selected, events); selected != chart.selected {
				chart.selected = selected
				gtx.Execute(op.InvalidateCmd{})
			}
			return dims
		}))
		rows = append(rows, ui.Rigid(chart.table.layout(view, len(donut.Slices), func() charts.Table {
			donut.Selected = chart.selected
			return donut.Table(chartTableLabels(l, i18n.NoMessage))
		})))
		return overviewBreakdownCard(gtx, view, rows)
	}
}

func (p *portfolioPage) portfolioAllocationAccount(view ui.View, value domain.AccountValuation) layout.Widget {
	l, account := view.Language, value.Account
	name := view.Body(ui.TextLabel, account.Name, view.Colors.Ink)
	if p.openAccount != nil {
		name = p.holdingAccountLink(view, portfolioButton(p.accounts, account.ID), account.ID, account.Name)
	}
	description := l.Text(accountTypes[account.AccountType])
	if value.InstitutionName != "" {
		description += " · " + value.InstitutionName
	}
	amount := l.Text(i18n.OverviewNoValue)
	if value.BaseValue != nil {
		amount = valuationMoney(view, *value.BaseValue, false)
	}
	return func(gtx layout.Context) layout.Dimensions {
		return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
			accountRows := func(gtx layout.Context) layout.Dimensions {
				return (ui.Column{Gap: ui.Space4}).Layout(gtx, ui.Rigid(name), ui.Rigid(view.Body(ui.TextCaption, description, view.Colors.SecondaryInk)))
			}
			figures := func(gtx layout.Context) layout.Dimensions {
				rows := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextLabel, amount, view.Colors.Ink))}
				if !value.Complete {
					rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
						return view.Badge(gtx, ui.BadgeWarning, l.Text(i18n.OverviewIncomplete))
					}))
				}
				return (ui.Column{Gap: ui.Space4}).Layout(gtx, rows...)
			}
			return (ui.Grid{MinColumn: 148, MaxColumns: 2, Gap: ui.Space12}).Layout(gtx, view, 2,
				func(gtx layout.Context, _ ui.View, i int) layout.Dimensions {
					if i == 0 {
						return accountRows(gtx)
					}
					return layout.E.Layout(gtx, figures)
				})
		})
	}
}
