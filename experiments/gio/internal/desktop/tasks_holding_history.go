package desktop

import (
	"fmt"
	"slices"
	"time"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func (p *tasksPage) holdingChangeRuntime() investmentRuntime {
	return investmentRuntime{ctx: p.ctx, workers: &p.workers, clock: p.holdingRecon.clock,
		invalidate: p.invalidate, refreshLink: p.readHoldingReconciliation}
}

type linkedHoldingMode uint8

const (
	linkedHoldingBuy linkedHoldingMode = iota
	linkedHoldingSell
	linkedHoldingDividend
	linkedHoldingTransfer
	linkedHoldingAdjustment
)

var linkedHoldingLabels = [...]i18n.MessageID{
	i18n.HistoryBuy, i18n.HistorySell, i18n.HistoryCashDividend, i18n.CustodyOpen, i18n.QuantityAdjustOpen,
}

type holdingLinkedActivity struct {
	scope      positionScope
	link       func() *domain.ReconciliationLink
	mode       linkedHoldingMode
	buttons    [len(linkedHoldingLabels)]widget.Clickable
	importOnly bool
}

func (c *holdingReconciliation) datedCheckpointLink(accountID string, item domain.ReconciliationItem) func() *domain.ReconciliationLink {
	sessionID := c.id
	return func() *domain.ReconciliationLink {
		a, current, found := c.checkpoint(accountID, item.ID, false)
		if !found || c.id != sessionID || c.session.Status != "in_progress" ||
			current.Kind != item.Kind || current.Currency != item.Currency ||
			current.HoldingID != item.HoldingID || current.InstrumentID != item.InstrumentID ||
			current.ArchivedNonzero || current.ProductID != "" {
			return nil
		}
		return &domain.ReconciliationLink{SessionID: sessionID, AccountID: a.AccountID, ItemID: current.ID,
			ExpectedRevision: c.session.Revision, FinancialRevision: a.FinancialRevision}
	}
}

func (p *tasksPage) startHoldingDatedChange(account domain.ReconciliationAccount, item domain.ReconciliationItem, entry *widget.Clickable) {
	c := p.holdingRecon
	at := slices.IndexFunc(c.directory.accounts, func(a domain.Account) bool { return a.ID.String() == account.AccountID })
	if at < 0 {
		return
	}
	linked := &holdingLinkedActivity{
		scope:      positionScope{account: c.directory.accounts[at], action: entry},
		link:       c.datedCheckpointLink(account.AccountID, item),
		importOnly: item.Kind == "scope",
	}
	if !linked.importOnly {
		id, err := domain.ParseHoldingID(item.HoldingID)
		instrument, instrumentErr := domain.ParseInstrumentID(item.InstrumentID)
		if err != nil || instrumentErr != nil {
			return
		}
		linked.scope.holding = domain.Holding{ID: id, InstrumentID: instrument}
	}
	c.saved = i18n.NoMessage
	c.dated = &accountInvestment{service: c.ledger, linked: linked, onPosted: func(domain.Activity, i18n.MessageID, event.Tag) {
		p.finishDatedCheckpoint(entry, true)
	}}
	run := p.holdingChangeRuntime()
	if linked.importOnly {
		c.dated.draft = newAccountInvestmentDraft(domain.ActivityPositionTransfer, linked.scope.account)
		c.dated.draft.link = linked.link
		c.dated.loadSources(run)
	} else {
		c.dated.selectLinkedHoldingMode(run, linkedHoldingBuy)
	}
}

func (p *tasksPage) finishDatedCheckpoint(entry *widget.Clickable, posted bool) {
	c := p.holdingRecon
	c.restore, c.restoreEditScroll = entry, true
	if posted {
		c.saved = i18n.HoldingReconDatedSaved
		p.readHoldingReconciliation()
		p.read()
	}
}

func (f *accountInvestment) selectLinkedHoldingMode(run investmentRuntime, mode linkedHoldingMode) {
	linked, previous := f.linked, f.draft
	linked.mode = mode
	switch mode {
	case linkedHoldingAdjustment:
		f.startAdjustment(run, linked.scope)
	case linkedHoldingTransfer:
		f.startCustody(run, linked.scope)
	default:
		kinds := [...]domain.ActivityKind{domain.ActivityBuy, domain.ActivitySell, domain.ActivityCashDividend}
		f.draft = newAccountInvestmentDraft(kinds[mode], linked.scope.account)
		f.draft.link, f.draft.lockedHolding = linked.link, linked.scope.holding
		f.loadSources(run)
	}
	f.draft.link = linked.link
	if previous != nil {
		f.draft.day, f.draft.time, f.draft.defaultTime = previous.day, previous.time, previous.defaultTime
		f.draft.note.Editor.SetText(previous.note.Text())
	}
}

func (p *tasksPage) layoutHoldingDatedChange(gtx layout.Context, view ui.View) layout.Dimensions {
	f := p.holdingRecon.dated
	run := p.holdingChangeRuntime()
	draft := f.draft
	if draft.review != nil || f.linked.importOnly {
		return f.layoutForm(gtx, view, run)
	}
	dims, next := layoutDatedCheckpointForm(gtx, view, linkedHoldingLabels[:], f.linked.buttons[:],
		int(f.linked.mode), draft.previewing != nil, func(gtx layout.Context, view ui.View) layout.Dimensions {
			return f.layoutForm(gtx, view, run)
		})
	if f.draft == draft && draft.previewing == nil && linkedHoldingMode(next) != f.linked.mode {
		f.selectLinkedHoldingMode(run, linkedHoldingMode(next))
		view.RestorePageScroll(0)
		gtx.Execute(op.InvalidateCmd{})
	}
	return dims
}

func layoutDatedCheckpointForm(gtx layout.Context, view ui.View, labels []i18n.MessageID, buttons []widget.Clickable,
	selected int, busy bool, form func(layout.Context, ui.View) layout.Dimensions) (layout.Dimensions, int) {
	origin, next := view.Origin, selected
	dims := layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, func(gtx layout.Context) layout.Dimensions {
				return (ui.Wrap{Gap: 6}).Layout(gtx, len(labels), func(gtx layout.Context, i int) layout.Dimensions {
					button := &buttons[i]
					if busy {
						gtx = gtx.Disabled()
					} else if button.Clicked(gtx) {
						next = i
					}
					return view.TabButton(button, view.Language.Text(labels[i]), selected == i)(gtx)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			view.Origin = origin
			return form(gtx, view)
		}))
	return dims, next
}

type checkpointCashScope struct {
	account  domain.AccountID
	currency domain.CurrencyCode
	entry    *widget.Clickable
	link     func() *domain.ReconciliationLink
	refresh  func()
	buttons  [2]widget.Clickable
}

var checkpointCashModes = [...]history.CashMovement{history.CashContribution, history.CashWithdrawal}
var checkpointCashLabels = [...]i18n.MessageID{i18n.HistoryCashIn, i18n.HistoryCashOut}

func (p *tasksPage) startDatedCheckpointCash(account domain.ReconciliationAccount, item domain.ReconciliationItem, entry *widget.Clickable) {
	c := p.holdingRecon
	id, err := domain.ParseAccountID(account.AccountID)
	if err != nil {
		return
	}
	f := newHistoryCashMovements(p.ctx, c.ledger, c.clock, p.invalidate)
	f.linked = &checkpointCashScope{account: id, currency: domain.CurrencyCode(item.Currency), entry: entry,
		link: c.datedCheckpointLink(account.AccountID, item), refresh: p.readHoldingReconciliation}
	c.saved, c.cashDated = i18n.NoMessage, f
	f.start(history.CashContribution)
}

func (p *tasksPage) layoutDatedCheckpointCash(gtx layout.Context, view ui.View) layout.Dimensions {
	f := p.holdingRecon.cashDated
	draft := f.draft
	if draft.review != nil {
		return f.layoutForm(gtx, view)
	}
	selected := slices.Index(checkpointCashModes[:], draft.movement)
	dims, next := layoutDatedCheckpointForm(gtx, view, checkpointCashLabels[:], f.linked.buttons[:], selected,
		draft.previewing != nil, f.layoutForm)
	if f.draft == draft && draft.previewing == nil && next != selected {
		f.start(checkpointCashModes[next])
		f.draft.day, f.draft.time, f.draft.defaultTime = draft.day, draft.time, draft.defaultTime
		f.draft.note.Editor.SetText(draft.note.Text())
		view.RestorePageScroll(0)
		gtx.Execute(op.InvalidateCmd{})
	}
	return dims
}

func holdingLinkedReceiptLabel(view ui.View, zone *time.Location, evidence holdingLinkedEvidence) string {
	l := view.Language
	label := l.Text(i18n.HoldingReconViewActivity)
	if zone != nil && !evidence.Committed.EffectiveAt.IsZero() {
		label = fmt.Sprintf(l.Text(i18n.HoldingReconViewDatedActivity),
			historyDay(view, evidence.Committed.EffectiveAt.In(zone).Format(time.DateOnly)))
	}
	if note := evidence.Committed.Note; note != nil && *note != "" {
		label += " · " + *note
	}
	return label
}

func (p *tasksPage) holdingReceiptLinks(view ui.View, accountID, itemID string) []layout.Widget {
	c, l := p.holdingRecon, view.Language
	if p.openActivity == nil {
		return nil
	}
	itemLinks := c.activityLinks[checkpointReceiptGroup{accountID, itemID}]
	var rows []layout.Widget
	limited := len(itemLinks) > reconciliationJournalPageSize
	if limited {
		itemLinks = itemLinks[len(itemLinks)-5:]
	}
	for _, link := range itemLinks {
		id, err := domain.ParseActivityID(link.OperationID)
		if err != nil {
			continue
		}
		var evidence holdingLinkedEvidence
		decoded := jsoncodec.Unmarshal(link.Evidence, &evidence) == nil
		label := holdingLinkedReceiptLabel(view, c.zone, evidence)
		button := portfolioButton(c.receiptButtons, id)
		rows = append(rows, taskLink(view, button, label, func() { p.openActivity(id, button) }))
		if decoded && evidence.Before != nil {
			d := c.audit[id]
			if d == nil {
				d = new(holdingEvidenceDisclosure)
				c.audit[id] = d
			}
			rows = append(rows, d.toggle(view, l.Text(i18n.HoldingReconShowAudit), l.Text(i18n.HoldingReconHideAudit)))
			if d.open {
				rows = append(rows, holdingLinkedEvidenceRows(view, evidence, c.zone, c.accountRoles)...)
			}
		}
	}
	if limited {
		rows = append(rows, view.Message(l.Text(i18n.HoldingReconMoreReceipts), view.Colors.SecondaryInk))
	}
	return rows
}
