package desktop

import (
	"fmt"
	"slices"
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
)

type overviewAssetLink struct {
	id    domain.AccountID
	label string
}

type overviewAllocationRow struct {
	id                 charts.ID
	class, institution string
	account            overviewAssetLink
	amount             string
}

type overviewAllocationModel struct {
	spec     charts.SankeySpec
	rows     []overviewAllocationRow
	accounts map[charts.ID]overviewAssetLink
}

type overviewAllocation struct {
	chart                               charts.SankeyChart
	selected                            charts.Selection
	table                               chartDataTable
	buttons                             map[string]*widget.Clickable
	read                                *overviewRead
	language                            i18n.Language
	decimalSeparator, groupingSeparator string
	model                               overviewAllocationModel
	openAccount                         func(domain.AccountID, string, event.Tag)
}

func (a *overviewAllocation) update(view ui.View, read *overviewRead) {
	if a.read == read && a.language == view.Language && a.decimalSeparator == view.DecimalSeparator && a.groupingSeparator == view.GroupingSeparator {
		return
	}
	if a.read != read {
		a.selected = charts.Selection{}
	}
	a.read, a.language = read, view.Language
	a.decimalSeparator, a.groupingSeparator = view.DecimalSeparator, view.GroupingSeparator
	a.model = overviewAssetAllocation(view, read.value)
}

func overviewAssetAllocation(view ui.View, data domain.OverviewResult) overviewAllocationModel {
	l, composition := view.Language, data.AssetComposition
	root := i18n.OverviewAllocationTotal
	switch {
	case composition.Negative.IsNegative():
		root = i18n.OverviewAllocationPositive
	case composition.MissingCount > 0:
		root = i18n.OverviewKnownSubtotal
	}
	out := overviewAllocationModel{
		spec: charts.SankeySpec{
			Label:        l.Text(i18n.OverviewAllocationTitle),
			ColumnTitles: []string{l.Text(root), l.Text(i18n.OverviewAllocationClass), l.Text(i18n.AccountMaintenanceInstitution), l.Text(i18n.OverviewAllocationAccount)},
			Messages:     charts.Messages{Empty: l.Text(i18n.OverviewAllocationEmpty), Missing: l.Text(i18n.OverviewNoValue), Selected: l.Text(i18n.ChartSelected)},
		},
		accounts: make(map[charts.ID]overviewAssetLink),
	}
	accountLabels, institutionLabels := overviewPathLabels(l, composition.Paths)
	for _, node := range composition.Nodes {
		id := charts.ID(node.ID)
		name := l.Text(i18n.OverviewEvidenceUnavailable)
		switch node.Kind {
		case "total":
			name = l.Text(root)
		case "assetClass":
			name = wealthAllocationName(l, node.Key, false)
		case "institution":
			if label, ok := institutionLabels[node.Key]; ok {
				name = label
			}
		case "account":
			if label, ok := accountLabels[node.Key]; ok {
				name = label
			}
			if node.Key != "" {
				out.accounts[id] = overviewAssetLink{id: domain.AccountID(node.Key), label: name}
			}
		}
		money := overviewAllocationMoney(view, data.Currency, node.Amount)
		out.spec.Nodes = append(out.spec.Nodes, charts.SankeyNode{ID: id, Label: name, Text: money, HideLabel: node.ParentID == ""})
		if node.ParentID != "" {
			link := charts.ID("/") + id
			amount := charts.NoValue
			if node.Amount.IsPositive() {
				value := node.Amount.InexactFloat64()
				if value > 0 {
					amount = charts.Num(value)
				}
			}
			out.spec.Links = append(out.spec.Links, charts.SankeyLink{ID: link, Source: charts.ID(node.ParentID), Target: id, Value: amount, Text: money})
			if target, ok := out.accounts[id]; ok {
				out.accounts[link] = target
			}
		}
	}
	for _, path := range composition.Paths {
		id := strings.Join([]string{path.AssetClass, path.InstitutionID, path.AccountID}, "/")
		out.rows = append(out.rows, overviewAllocationRow{
			id: charts.ID(id), class: wealthAllocationName(l, path.AssetClass, false), institution: institutionLabels[path.InstitutionID],
			account: overviewAssetLink{id: domain.AccountID(path.AccountID), label: accountLabels[path.AccountID]},
			amount:  overviewAllocationMoney(view, data.Currency, path.Amount),
		})
	}
	return out
}

func overviewPathLabels(l i18n.Language, paths []domain.OverviewAssetPath) (accounts, institutions map[string]string) {
	accounts, institutions = map[string]string{}, map[string]string{}
	for _, path := range paths {
		account := path.AccountName
		if account == "" {
			account = l.Text(i18n.OverviewAllocationUnknownAccount)
		}
		accounts[path.AccountID] = account
		institution := path.InstitutionName
		if path.InstitutionID == "" {
			institution = l.Text(i18n.OverviewAllocationUnassigned)
		} else if institution == "" {
			institution = l.Text(i18n.OverviewEvidenceUnavailable)
		}
		institutions[path.InstitutionID] = institution
	}
	overviewDistinctPathLabels(l, accounts)
	overviewDistinctPathLabels(l, institutions)
	return accounts, institutions
}

func overviewDistinctPathLabels(l i18n.Language, labels map[string]string) {
	groups := make(map[string][]string)
	used := make(map[string]bool)
	var names []string
	for id, label := range labels {
		if !used[label] {
			names = append(names, label)
		}
		used[label] = true
		groups[label] = append(groups[label], id)
	}
	slices.Sort(names)
	for _, name := range names {
		ids := groups[name]
		if len(ids) < 2 {
			continue
		}
		slices.Sort(ids)
		ordinal := 0
		for _, id := range ids {
			if id == "" {
				continue
			}
			for {
				ordinal++
				label := fmt.Sprintf(l.Text(i18n.OverviewAllocationOrdinal), name, ordinal)
				if !used[label] {
					used[label], labels[id] = true, label
					break
				}
			}
		}
	}
}

func overviewAllocationMoney(view ui.View, currency domain.CurrencyCode, amount decimal.Decimal) string {
	return valuationMoney(view, domain.MoneyView{Currency: currency, Amount: amount.String()}, false)
}

func (a *overviewAllocation) section(gtx layout.Context, view ui.View, data domain.OverviewResult) layout.Dimensions {
	l, c := view.Language, data.AssetComposition
	title := func(gtx layout.Context) layout.Dimensions {
		return view.Line(gtx, ui.TextHeading, l.Text(i18n.OverviewAllocationTitle), view.Colors.Ink)
	}
	rows := []ui.ColumnChild{ui.Rigid(title)}
	rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.OverviewAllocationScope), view.Colors.SecondaryInk)))
	appendAmount := func(id i18n.MessageID, amount decimal.Decimal) {
		rows = append(rows, ui.Rigid(view.Body(ui.TextLabel, l.Text(id)+" · "+overviewAllocationMoney(view, data.Currency, amount), view.Colors.Ink)))
	}
	valued := len(c.Paths) != 0 || c.Negative.IsNegative() || c.ZeroCount > 0
	if c.Negative.IsNegative() {
		appendAmount(i18n.OverviewAllocationPositive, c.Positive)
		appendAmount(i18n.OverviewAllocationNegative, c.Negative)
	}
	if valued || c.MissingCount == 0 {
		id := i18n.OverviewAllocationTotal
		if c.MissingCount > 0 {
			id = i18n.OverviewKnownSubtotal
		}
		appendAmount(id, data.Assets)
	}
	if c.MissingCount > 0 {
		rows = append(rows, ui.Rigid(view.Body(ui.TextCaption, fmt.Sprintf(l.Text(i18n.OverviewAllocationMissing), c.MissingCount), view.Colors.SecondaryInk)))
	}
	if c.ZeroCount > 0 {
		rows = append(rows, ui.Rigid(view.Body(ui.TextCaption, fmt.Sprintf(l.Text(i18n.OverviewAllocationZero), c.ZeroCount), view.Colors.SecondaryInk)))
	}
	if len(c.Paths) == 0 {
		id := i18n.OverviewAllocationEmpty
		if !valued && c.MissingCount > 0 {
			id = i18n.OverviewAllocationUnavailable
		}
		rows = append(rows, ui.Rigid(view.Message(l.Text(id), view.Colors.SecondaryInk)))
	} else {
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			view.Focus.Watch(gtx, &a.chart)
			spec := a.model.spec
			spec.Selected = a.selected
			dims, events := a.chart.Layout(gtx, view, spec)
			if len(events) > 0 {
				next := charts.Apply(a.selected, events)
				if account, ok := a.model.accounts[next.Item]; ok && a.openAccount != nil {
					a.openAccount(account.id, account.label, &a.chart)
				} else {
					a.selected = next
				}
				gtx.Execute(op.InvalidateCmd{})
			}
			return dims
		}))
		rows = append(rows, ui.Rigid(a.table.layoutDisclosure(view, l.Text(i18n.OverviewAllocationDetails), len(a.model.rows),
			func() charts.Table { return a.pathTable(view) }, nil,
			func(index int) layout.Widget { return a.pathCell(view, index) })))
	}
	return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
		return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
	})
}

func (a *overviewAllocation) pathTable(view ui.View) charts.Table {
	l := view.Language
	table := charts.Table{Columns: []charts.TableColumn{
		{Title: l.Text(i18n.OverviewAllocationColumn), Wrap: true},
		{Title: l.Text(i18n.ChartAmount), Numeric: true},
	}, Selected: charts.NoRow}
	for i, row := range a.model.rows {
		table.Rows = append(table.Rows, []string{row.account.label + "\n" + row.class + " · " + row.institution, row.amount})
		if a.selected.Item == row.id || a.selected.Item == "/"+row.id {
			table.Selected = i
		}
	}
	return table
}

func (a *overviewAllocation) pathCell(view ui.View, index int) layout.Widget {
	row := a.model.rows[index]
	return func(gtx layout.Context) layout.Dimensions {
		context := view.Body(ui.TextCaption, row.class+" · "+row.institution, view.Colors.SecondaryInk)
		if row.account.id == "" || a.openAccount == nil {
			return (ui.Column{Gap: ui.Space2}).Layout(gtx,
				ui.Rigid(view.Body(ui.TextBody, row.account.label, view.Colors.Ink)), ui.Rigid(context))
		}
		if a.buttons == nil {
			a.buttons = make(map[string]*widget.Clickable)
		}
		button := portfolioButton(a.buttons, string(row.id))
		view.Focus.Watch(gtx, button)
		if button.Clicked(gtx) {
			a.openAccount(row.account.id, row.account.label, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		name := fmt.Sprintf(view.Language.Text(i18n.HistoryOpenAccount), row.account.label)
		return (ui.Column{Gap: ui.Space2}).Layout(gtx,
			ui.Rigid(view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonLink, Size: ui.ButtonSmall, Title: row.account.label, Name: name})),
			ui.Rigid(context))

	}
}
