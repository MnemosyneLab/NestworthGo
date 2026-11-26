package desktop

import (
	"fmt"
	"strings"

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

type accountCompositionVisit struct {
	detail, expanded, tableOpen, cashHistoryOpen bool
	selected                                     charts.Selection
	tablePosition, cashHistoryPosition           layout.Position
}

type accountCompositionControls struct {
	accountCompositionVisit
	types, details, toggle widget.Clickable
	chart                  overviewSummaryChart
}

func (p *accountsPage) accountComposition(view ui.View, composition *domain.AccountComposition) layout.Widget {
	if composition == nil || composition.Status == "unsupported" {
		return nil
	}
	d, l := &p.detail.composition, view.Language
	return func(gtx layout.Context) layout.Dimensions {
		if d.types.Clicked(gtx) && d.detail {
			d.detail, d.expanded, d.chart.selected = false, false, charts.Selection{}
			gtx.Execute(op.InvalidateCmd{})
		}
		if d.details.Clicked(gtx) && !d.detail {
			d.detail, d.expanded, d.chart.selected = true, false, charts.Selection{}
			gtx.Execute(op.InvalidateCmd{})
		}
		donut, remainder := accountCompositionSpec(view, composition, d.detail, d.expanded)
		rows := []ui.ColumnChild{
			ui.Rigid(view.Body(ui.TextHeading, donut.Label, view.Colors.Ink)),
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				return view.TabTrack(gtx, 2, func(gtx layout.Context, i int) layout.Dimensions {
					if i == 0 {
						return view.TabSegment(&d.types, l.Text(i18n.AccountCompositionType), !d.detail)(gtx)
					}
					return view.TabSegment(&d.details, l.Text(i18n.AccountCompositionDetail), d.detail)(gtx)
				})
			}),
			ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.AccountCompositionCurrency), composition.Currency), view.Colors.SecondaryInk)),
		}
		if !composition.Complete && composition.MissingCount > 0 {
			rows = append(rows, ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.OverviewCompositionMissing), composition.MissingCount), view.Colors.WarningInk)))
		}
		switch composition.Status {
		case "ready", "partial":
			rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				donut.Selected = d.chart.selected
				view.Focus.Watch(gtx, &d.chart.donut)
				dims, events := d.chart.donut.Layout(gtx, view, donut)
				if selection := charts.Apply(d.chart.selected, events); selection != d.chart.selected {
					d.chart.selected = selection
					gtx.Execute(op.InvalidateCmd{})
				}
				return dims
			}))
		default:
			status := i18n.AccountCompositionUnavailable
			if composition.Status == "empty" {
				status = i18n.AccountCompositionEmpty
			} else if composition.Status == "invalid" {
				status = i18n.AccountCompositionInvalid
			}
			rows = append(rows, ui.Rigid(view.Message(l.Text(status), view.Colors.SecondaryInk)))
			for _, row := range donut.Rows() {
				rows = append(rows, ui.Rigid(labeledRow(view, row.Label+" · "+row.Value, row.Label, row.Value, view.Colors.Ink)))
			}
		}
		rows = append(rows, ui.Rigid(view.Message(donut.Summary, view.Colors.SecondaryInk)),
			ui.Rigid(d.chart.table.layout(view, len(composition.Details), func() charts.Table {
				return accountCompositionTable(view, composition, d.chart.selected)
			})))
		if d.detail && remainder {
			label := fmt.Sprintf(l.Text(i18n.AccountCompositionExpand), len(composition.Details))
			if d.expanded {
				label = l.Text(i18n.AccountCompositionCollapse)
			}
			if d.toggle.Clicked(gtx) {
				d.expanded, d.chart.selected = !d.expanded, charts.Selection{}
				gtx.Execute(op.InvalidateCmd{})
			}
			rows = append(rows, ui.Rigid(analysisDisclosure(view, &d.toggle, label, d.expanded)))
		}
		return overviewBreakdownCard(gtx, view, rows)
	}
}

func accountCompositionSpec(view ui.View, c *domain.AccountComposition, detail, expanded bool) (charts.DonutSpec, bool) {
	l := view.Language
	spec := charts.DonutSpec{
		Compact: true, Label: l.Text(i18n.AccountCompositionTitle), Summary: l.Text(i18n.AccountCompositionHint),
		CenterCaption: l.Text(i18n.OverviewKnownSubtotal), CenterValue: accountCompositionMoney(view, c.TotalExact, c.Currency),
		NotDrawn: l.Text(i18n.WealthAllocationNotDrawn), ZeroTotal: l.Text(i18n.AccountCompositionEmpty),
		Messages: charts.Messages{Missing: l.Text(i18n.OverviewNoValue), Selected: l.Text(i18n.ChartSelected), Empty: l.Text(i18n.AccountCompositionUnavailable)},
	}
	items := c.Types
	remainder := false
	if detail {
		items = c.DetailSlices
		for _, item := range items {
			remainder = remainder || isAccountCompositionRemainder(item)
		}
		if expanded {
			items = c.Details
		}
	}
	for _, item := range items {
		value := charts.NoValue
		if amount, err := decimal.NewFromString(item.BaseAmountExact); err == nil && item.ValuedCount > 0 {
			value = charts.Num(amount.InexactFloat64())
		}
		label := accountCompositionDetailLabel(l, item)
		if !detail {
			label = accountCompositionTypeLabel(l, item.TypeKey)
		} else if isAccountCompositionRemainder(item) {
			label = fmt.Sprintf(l.Text(i18n.AccountCompositionRemainder), len(item.DetailRefs))
		}
		row := charts.Slice{ID: charts.ID(item.Key), Label: label, Value: value,
			Text: accountCompositionMoney(view, item.BaseAmountExact, c.Currency), Share: accountCompositionShare(view, c.Status, item)}
		if detail && item.NativeAmount != "" && item.NativeCurrency != c.Currency {
			row.Detail = append(row.Detail, accountCompositionMoney(view, item.NativeAmount, item.NativeCurrency))
		}
		if item.MissingCount > 0 {
			row.Detail = append(row.Detail, fmt.Sprintf(l.Text(i18n.OverviewCompositionMissing), item.MissingCount))
		}
		spec.Slices = append(spec.Slices, row)
	}
	return spec, remainder
}

func isAccountCompositionRemainder(item domain.AccountCompositionItem) bool {
	return item.Key == "account-composition:remainder"
}

func accountCompositionDetailLabel(l i18n.Language, item domain.AccountCompositionItem) string {
	if item.Label == "" {
		return l.Text(i18n.OverviewEvidenceUnavailable)
	}
	if item.IdentityLabel == "" {
		if _, err := domain.ParseInstrumentID(item.Label); err == nil {
			return l.Text(i18n.OverviewEvidenceUnavailable)
		}
	}
	return item.Label
}

func accountCompositionTypeLabel(l i18n.Language, key string) string {
	switch key {
	case "cash":
		return l.Text(i18n.AccountCash)
	case "bank_investment_product":
		return l.Text(i18n.AccountCompositionProducts)
	case "unknown":
		return l.Text(i18n.AccountCompositionUnknown)
	default:
		return wealthAllocationName(l, key, false)
	}
}

func accountCompositionMoney(view ui.View, amount string, currency domain.CurrencyCode) string {
	if amount == "" {
		return view.Language.Text(i18n.OverviewNoValue)
	}
	return valuationMoney(view, domain.MoneyView{Amount: amount, Currency: currency}, false)
}

func accountCompositionShare(view ui.View, status string, item domain.AccountCompositionItem) string {
	if item.ValuedCount == 0 || status != "ready" && status != "partial" && status != "empty" {
		return "—"
	}
	return portfolioDecimal(view, strings.TrimSuffix(format.ShareBPS(item.ShareBPS), "%")) + "%"
}

func accountCompositionTable(view ui.View, c *domain.AccountComposition, selection charts.Selection) charts.Table {
	l := view.Language
	table := charts.Table{Selected: charts.NoRow, Columns: []charts.TableColumn{
		{Title: l.Text(i18n.ChartCategory), Wrap: true},
		{Title: l.Text(i18n.AccountCompositionNative), Numeric: true},
		{Title: l.Text(i18n.AccountCompositionBase), Numeric: true},
		{Title: l.Text(i18n.ChartShare), Numeric: true},
	}}
	for _, item := range c.Details {
		label := item.IdentityLabel
		if label == "" {
			label = accountCompositionDetailLabel(l, item)
		}
		if selection.Item == charts.ID(item.Key) {
			table.Selected = len(table.Rows)
		}
		table.Rows = append(table.Rows, []string{label,
			accountCompositionMoney(view, item.NativeAmount, item.NativeCurrency),
			accountCompositionMoney(view, item.BaseAmountExact, c.Currency),
			accountCompositionShare(view, c.Status, item),
		})
	}
	return table
}
