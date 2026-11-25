package desktop

import (
	"cmp"
	"fmt"
	"slices"

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

type wealthChange struct {
	liabilities, all                 bool
	assets, debt, showAll, reconcile widget.Clickable
	list                             widget.List
	chart                            charts.BarChart
}

type wealthChangedAccount struct {
	row    application.HistoricalOverviewRow
	amount decimal.Decimal
}

func wealthRowDelta(row application.HistoricalOverviewRow) *string {
	if row.Left != nil && row.Right != nil && row.Left.DisplayValuation.Quality.Availability != "" && row.Right.DisplayValuation.Quality.Availability != "" {
		return row.EstimatedBaseChange
	}
	return row.BaseChange
}

func wealthStatusComparable(a, b string) bool {
	return a == b || (a == "active" || a == "zero") && (b == "active" || b == "zero")
}

func wealthChangeAccounts(result *application.HistoricalOverviewResult, liability bool) (changed []wealthChangedAccount, separate []application.HistoricalOverviewRow) {
	role := domain.RoleAsset
	if liability {
		role = domain.RoleLiability
	}
	for _, row := range result.Rows {
		if row.Kind != "account" || row.Role != string(role) {
			continue
		}
		if result.Left.Currency != result.Right.Currency || row.Left == nil || row.Right == nil || !row.Left.Included || !row.Right.Included ||
			!wealthStatusComparable(row.Left.Status, row.Right.Status) || !wealthFullAmount(row.Left) || !wealthFullAmount(row.Right) {
			separate = append(separate, row)
			continue
		}
		delta := wealthRowDelta(row)
		if delta == nil {
			separate = append(separate, row)
			continue
		}
		amount, err := decimal.NewFromString(*delta)
		if err != nil {
			separate = append(separate, row)
		} else if row.Changed {
			changed = append(changed, wealthChangedAccount{row: row, amount: amount})
		}
	}
	slices.SortStableFunc(changed, func(a, b wealthChangedAccount) int {
		return cmp.Or(b.amount.Abs().Cmp(a.amount.Abs()), cmp.Compare(a.row.Key, b.row.Key))
	})
	return changed, separate
}

func wealthFullAmount(cell *application.HistoricalOverviewCell) bool {
	if cell.DisplayValuation.Quality.Availability != "" {
		return cell.DisplayValuation.Total != nil
	}
	return cell.Complete && cell.BaseAmount != nil
}

func wealthChangeBars(view ui.View, accounts []wealthChangedAccount, currency string, debt bool) charts.BarSpec {
	l := view.Language
	spec := charts.BarSpec{Label: l.Text(i18n.WealthChangeTitle), Summary: l.Text(i18n.WealthChangeBasis),
		Messages: charts.Messages{Missing: l.Text(i18n.OverviewNoValue), Selected: l.Text(i18n.ChartSelected)}}
	for _, item := range accounts {
		amount, tone := item.amount, charts.ToneAuto
		if debt {
			amount, tone = amount.Neg(), charts.ToneNeutral
		}
		canonical := item.amount.String()
		spec.Bars = append(spec.Bars, charts.Bar{ID: charts.ID(item.row.Key), Label: wealthRowName(l, item.row),
			Value: wealthChartValue(amount.String(), false), Text: wealthExactMoney(view, &canonical, currency, debt, true), Tone: tone})
	}
	return spec
}

func (p *wealthHistoryPage) changeRows(gtx layout.Context, view ui.View, result *application.HistoricalOverviewResult) []ui.ColumnChild {
	c, l := &p.change, view.Language
	assets, debt := c.assets.Clicked(gtx), c.debt.Clicked(gtx)
	if assets || debt {
		c.liabilities = debt
		c.all = false
		c.list.Position = layout.Position{}
		gtx.Execute(op.InvalidateCmd{})
	}
	if c.showAll.Clicked(gtx) {
		c.all = !c.all
		gtx.Execute(op.InvalidateCmd{})
	}
	children := []ui.ColumnChild{
		ui.Rigid(view.Body(ui.TextLabel, l.Text(i18n.WealthChangeTitle), view.Colors.Ink)),
		ui.Rigid(view.Message(l.Text(i18n.WealthChangeBasis), view.Colors.SecondaryInk)),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return (ui.Wrap{Gap: 8}).Layout(gtx, 2, func(gtx layout.Context, i int) layout.Dimensions {
				if i == 0 {
					return analysisToggle(view, &c.assets, l.Text(i18n.Assets), !c.liabilities)(gtx)
				}
				return analysisToggle(view, &c.debt, l.Text(i18n.Liabilities), c.liabilities)(gtx)
			})
		}),
	}
	mismatch := result.Left.Currency != result.Right.Currency
	if mismatch {
		children = append(children, ui.Rigid(view.Message(l.Text(i18n.WealthAllocationCurrencyChanged), view.Colors.SecondaryInk)))
	}
	changed, separate := wealthChangeAccounts(result, c.liabilities)
	shown := len(changed)
	if !c.all {
		shown = min(shown, 10)
	}
	if shown > 0 {
		spec := wealthChangeBars(view, changed[:shown], result.Left.Currency, c.liabilities)
		children = append(children, ui.Rigid(p.changeChart(view, result, changed[:shown], spec)))
	}
	children = append(children, ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.WealthChangeCount), shown, len(changed)), view.Colors.SecondaryInk)))
	if len(changed) > 10 {
		id := i18n.WealthChangeShowAll
		if c.all {
			id = i18n.WealthChangeTopTen
		}
		children = append(children, ui.Rigid(returnMonthButton(view, &c.showAll, id, nil, true)))
	}
	if len(changed)+len(separate) == 0 {
		return append(children, ui.Rigid(view.Message(l.Text(i18n.WealthChangeEmpty), view.Colors.SecondaryInk)))
	}
	return append(children, ui.Fill(func(gtx layout.Context) layout.Dimensions {
		count := len(changed) + len(separate)
		if len(separate) > 0 {
			count++
		}
		return analysisResultList(gtx, view, &c.list, l.Text(i18n.WealthChangeTitle), count, func(gtx layout.Context, i int) layout.Dimensions {
			switch {
			case i < len(changed):
				return p.accountRow(gtx, view, result, changed[i].row, false)
			case i == len(changed):
				return view.Body(ui.TextLabel, l.Text(i18n.WealthChangeSeparate), view.Colors.Ink)(gtx)
			default:
				return p.accountRow(gtx, view, result, separate[i-len(changed)-1], false)
			}
		})
	}))
}

func (p *wealthHistoryPage) changeChart(view ui.View, result *application.HistoricalOverviewResult, accounts []wealthChangedAccount, spec charts.BarSpec) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
			chart := &p.change.chart
			p.watchWealthDetailFocus(gtx, view, chart)
			dims, events := chart.Layout(gtx, view, spec)
			selected := charts.Apply(charts.Selection{}, events)
			for _, item := range accounts {
				if charts.ID(item.row.Key) == selected.Item {
					p.openWealthDetail(result, &item.row, chart)
					gtx.Execute(op.InvalidateCmd{})
					break
				}
			}
			return dims
		})
	}
}
