package desktop

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type accountDetailControls struct {
	openReconciliation func(domain.AccountID, string, string, event.Tag)
	openActivityList   func(domain.AccountID, event.Tag)
	openAnalysis       func(routeID, domain.AccountID, string, event.Tag)
	reconcile, receipt widget.Clickable
	record             accountRecordControls
	cashActions        accountCashActions
	composition        accountCompositionControls
	cashHistory        chartDataTable
	cashHistoryRetry   widget.Clickable

	investmentActionsRow int
	productRows          map[*widget.Clickable]int
	revealedShortcut     event.Tag
}

func (d *accountDetailControls) restore(visit accountCompositionVisit) {
	d.composition.accountCompositionVisit = visit
	d.composition.chart.selected = visit.selected
	d.composition.chart.table.open = visit.tableOpen
	d.composition.chart.table.list.Position = visit.tablePosition
	d.cashHistory.list.Position = visit.cashHistoryPosition
}

func (d *accountDetailControls) visit() accountCompositionVisit {
	visit := d.composition.accountCompositionVisit
	visit.selected = d.composition.chart.selected
	visit.tableOpen, visit.tablePosition = d.composition.chart.table.open, d.composition.chart.table.list.Position
	visit.cashHistoryPosition = d.cashHistory.list.Position
	return visit
}

func accountDetailButton(view ui.View, button *widget.Clickable, label i18n.MessageID, variant ui.ButtonVariant, action func()) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if button.Clicked(gtx) {
			action()
			gtx.Execute(op.InvalidateCmd{})
		}
		view.Focus.Watch(gtx, button)
		return view.ButtonOf(button, ui.ButtonSpec{Variant: variant, Size: ui.ButtonSmall, Title: view.Language.Text(label)})(gtx)
	}
}

func accountDetailActions(view ui.View, actions []layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return (ui.Wrap{Gap: ui.Space8, Middle: true}).Layout(gtx, len(actions), func(gtx layout.Context, i int) layout.Dimensions {
			return actions[i](gtx)
		})
	}
}

func (p *accountsPage) accountReconciliationRow(view ui.View, account domain.Account) layout.Widget {
	l, d, data := view.Language, &p.detail, p.current
	status, ink := l.Text(i18n.AccountCheckNever), view.Colors.Ink
	last := data.lastCheck
	switch {
	case data.checkErr != nil:
		status, ink = l.Text(i18n.AccountCheckLoadFailed), view.Colors.ErrorInk
	case last != nil:
		status = statementRecordedTime(view, last.CompletedAt)
	}
	heading := view.Body(ui.TextLabel, l.Text(i18n.AccountCheckLastSuccess)+": "+status, ink)
	var actions []layout.Widget
	if d.openReconciliation != nil {
		if account.ArchivedAt == nil {
			actions = append(actions, accountDetailButton(view, &d.reconcile, i18n.AccountCheckOpen, ui.ButtonOutline, func() {
				d.openReconciliation(account.ID, "", account.Name, &d.reconcile)
			}))
		}
		if last != nil {
			actions = append(actions, accountDetailButton(view, &d.receipt, i18n.AccountCheckLastRecord, ui.ButtonGhost, func() {
				d.openReconciliation(account.ID, last.ID, account.Name, &d.receipt)
			}))
		}
	}
	if f := p.statement; f != nil && statementCard(account) && p.freshDetail() != nil {
		actions = append(actions, func(gtx layout.Context) layout.Dimensions {
			view.Focus.Watch(gtx, &f.open)
			return view.ButtonOf(&f.open, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall,
				Title: l.Text(i18n.StatementOpen)})(gtx)
		})
	}
	rows := []layout.Widget{func(gtx layout.Context) layout.Dimensions {
		if !ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, 640) || len(actions) == 0 {
			return (ui.Column{Gap: ui.Space4}).Layout(gtx, ui.Rigid(heading), ui.Rigid(accountDetailActions(view, actions)))
		}
		return (layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}).Layout(gtx,
			layout.Flexed(1, heading), layout.Rigid(accountDetailActions(view, actions)))
	}}
	if last != nil {
		i := slices.IndexFunc(last.Accounts, func(row domain.ReconciliationAccount) bool {
			return row.AccountID == account.ID.String()
		})
		if i >= 0 {
			rows = append(rows, accountReconciliationEvidence(view, last.Accounts[i])...)
		}
	}
	return sectionCard(view, rows)
}

func accountReconciliationEvidence(view ui.View, check domain.ReconciliationAccount) []layout.Widget {
	l := view.Language
	var rows []layout.Widget
	if check.InventoryCoverageGap {
		rows = append(rows, view.Message(l.Text(i18n.AccountCheckHistoricalCoverageGap), view.Colors.WarningInk))
		for _, holding := range check.UncoveredArchivedHoldings {
			rows = append(rows, view.Message(holding.Label+" · "+portfolioDecimal(view, holding.Quantity), view.Colors.Ink))
		}
		rows = append(rows, view.Message(l.Text(i18n.AccountCheckNewCoverage), view.Colors.SecondaryInk))
	}
	if check.RecheckRequired {
		rows = append(rows, view.Message(l.Text(i18n.AccountCheckLaterBookkeeping), view.Colors.WarningInk))
	}
	return rows
}

func (p *accountsPage) accountCashHistory(view ui.View) layout.Widget {
	values := p.current.cashValues
	if p.current.cashValuesErr != nil {
		return sectionCard(view, []layout.Widget{
			view.Message(view.Language.Text(i18n.AccountCashHistoryLoadFailed), view.Colors.ErrorInk),
			accountDetailButton(view, &p.detail.cashHistoryRetry, i18n.HistoryRetry, ui.ButtonOutline, p.refresh),
		})
	}
	if len(values) == 0 {
		return nil
	}
	d, l := &p.detail.cashHistory, view.Language
	title := l.Text(i18n.AccountCashHistoryTitle)
	return sectionCard(view, []layout.Widget{
		view.Body(ui.TextHeading, title, view.Colors.Ink),
		func(gtx layout.Context) layout.Dimensions {
			table := accountCashHistoryTable(view, values)
			return d.tableWithCells(gtx, view, title, table, nil, func(row int) layout.Widget {
				return func(gtx layout.Context) layout.Dimensions {
					lines := strings.SplitN(table.Rows[row][0], "\n", 3)
					fields := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextBody, lines[0], view.Colors.Ink))}
					for _, line := range lines[1:] {
						fields = append(fields, ui.Rigid(view.Body(ui.TextCaption, line, view.Colors.SecondaryInk)))
					}
					return (ui.Column{Gap: ui.Space2}).Layout(gtx, fields...)
				}
			})
		},
	})
}

func accountCashHistoryTable(view ui.View, values []domain.AccountCashValue) charts.Table {
	l := view.Language
	table := charts.Table{Selected: charts.NoRow, Columns: []charts.TableColumn{
		{Title: l.Text(i18n.AccountCashHistoryType), Wrap: true},
		{Title: l.Text(i18n.AccountCashHistoryChange), Numeric: true},
		{Title: l.Text(i18n.AccountCashHistoryBalance), Numeric: true},
		{Title: l.Text(i18n.AccountCashHistoryCurrency)},
		{Title: l.Text(i18n.AccountCashHistoryEffective), Wrap: true},
		{Title: l.Text(i18n.AccountCashHistoryRecorded), Wrap: true},
	}}
	for _, value := range values {
		label := l.Text(i18n.AccountCashHistoryOpening)
		if value.ObservationKind == "event" {
			label = l.Text(i18n.AccountCashHistoryActivity)
		}
		if value.ActivityKind != nil {
			label = l.Text(historyKindLabel(*value.ActivityKind, ""))
		}
		if value.ActivityReason != nil {
			if reason := checkpointCashReasonLabel(*value.ActivityReason); reason != i18n.NoMessage {
				label += "\n" + l.Text(reason)
			}
		}
		if value.ActivityNote != nil && *value.ActivityNote != "" {
			label += "\n" + *value.ActivityNote
		}
		change := "—"
		if value.Change != nil {
			change = returnSignedMoney(view, value.Change)
		}
		balance := valuationMoney(view, domain.MoneyView{Amount: value.Amount.CanonicalAmount(), Currency: value.Amount.Currency()}, false)
		table.Rows = append(table.Rows, []string{label, change, balance, value.Amount.Currency().String(),
			statementRecordedTime(view, value.EffectiveAt.Format(time.RFC3339Nano)),
			statementRecordedTime(view, value.CreatedAt.Format(time.RFC3339Nano)),
		})
	}
	return table
}

func (p *accountsPage) accountDebtShortcut(view ui.View) layout.Widget {
	if !p.canPostLiability() {
		return nil
	}
	f := p.liabilityPosting
	return func(gtx layout.Context) layout.Dimensions {
		view.Focus.Watch(gtx, &f.open)
		if f.restore && gtx.Enabled() {
			gtx.Execute(key.FocusCmd{Tag: &f.open})
			f.restore = false
		}
		return view.ButtonOf(&f.open, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall,
			Title: view.Language.Text(i18n.LiabilityRecordDebt)})(gtx)
	}
}

func accountSimpleValue(view ui.View, row accountRow, actions []layout.Widget) layout.Widget {
	l := view.Language
	label := i18n.AccountCurrentBalance
	if row.record.Account.TrackingMode == domain.TrackingManualValue {
		label = i18n.AccountLastValuation
	}
	rows := []layout.Widget{
		view.Body(ui.TextHeading, l.Text(accountTypes[row.record.Account.AccountType]), view.Colors.Ink),
		view.Message(fmt.Sprintf(l.Text(label), accountAmountLines(view, row, false)[0].label), view.Colors.SecondaryInk),
	}
	if len(actions) > 0 {
		rows = append(rows, accountDetailActions(view, actions))
	}
	return sectionCard(view, rows)
}
