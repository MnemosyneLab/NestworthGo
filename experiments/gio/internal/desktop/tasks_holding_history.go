package desktop

import (
	"fmt"
	"slices"
	"time"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

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
	scope   positionScope
	link    func() *domain.ReconciliationLink
	mode    linkedHoldingMode
	buttons [len(linkedHoldingLabels)]widget.Clickable
}

func (p *tasksPage) startHoldingDatedChange(account domain.ReconciliationAccount, item domain.ReconciliationItem, entry *widget.Clickable) {
	c := p.holdingRecon
	at := slices.IndexFunc(c.directory.accounts, func(a domain.Account) bool { return a.ID.String() == account.AccountID })
	id, err := domain.ParseHoldingID(item.HoldingID)
	instrument, instrumentErr := domain.ParseInstrumentID(item.InstrumentID)
	if at < 0 || err != nil || instrumentErr != nil {
		return
	}
	linked := &holdingLinkedActivity{
		scope: positionScope{account: c.directory.accounts[at], holding: domain.Holding{ID: id, InstrumentID: instrument}, action: entry},
		link: func() *domain.ReconciliationLink {
			a, current, found := c.checkpoint(account.AccountID, item.ID, true)
			if !found || c.session.Status != "in_progress" || current.HoldingID != id.String() ||
				current.InstrumentID != instrument.String() || current.ArchivedNonzero || current.ProductID != "" {
				return nil
			}
			return &domain.ReconciliationLink{SessionID: c.id, AccountID: a.AccountID, ItemID: current.ID,
				ExpectedRevision: c.session.Revision, FinancialRevision: a.FinancialRevision}
		},
	}
	c.saved = i18n.NoMessage
	c.dated = &accountInvestment{service: c.ledger, linked: linked, onPosted: func(domain.Activity, i18n.MessageID, event.Tag) {
		c.saved, c.restore = i18n.HoldingReconDatedSaved, entry
		p.readHoldingReconciliation()
		p.read()
	}}
	c.dated.selectLinkedHoldingMode(p.holdingChangeRuntime(), linkedHoldingBuy)
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
	if f.draft.review != nil {
		return f.layoutForm(gtx, view, run)
	}
	origin := view.Origin
	draft, nextMode := f.draft, f.linked.mode
	dimensions := layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, func(gtx layout.Context) layout.Dimensions {
				return (ui.Wrap{Gap: 6}).Layout(gtx, len(linkedHoldingLabels), func(gtx layout.Context, i int) layout.Dimensions {
					mode, button := linkedHoldingMode(i), &f.linked.buttons[i]
					if f.draft.previewing != nil {
						gtx = gtx.Disabled()
					}
					if button.Clicked(gtx) && f.linked.mode != mode {
						nextMode = mode
					}
					return view.TabButton(button, view.Language.Text(linkedHoldingLabels[i]), f.linked.mode == mode)(gtx)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			view.Origin = origin
			return f.layoutForm(gtx, view, run)
		}))
	if f.draft == draft && draft.previewing == nil && nextMode != f.linked.mode {
		f.selectLinkedHoldingMode(run, nextMode)
		view.RestorePageScroll(0)
		gtx.Execute(op.InvalidateCmd{})
	}
	return dimensions
}

func (p *tasksPage) holdingReceiptLinks(view ui.View, accountID, itemID string) []layout.Widget {
	c, l := p.holdingRecon, view.Language
	if p.openActivity == nil {
		return nil
	}
	var rows []layout.Widget
	for _, link := range c.links {
		if link.Family != "activity" || link.AccountID != accountID || link.ItemID != itemID {
			continue
		}
		id, err := domain.ParseActivityID(link.OperationID)
		if err != nil {
			continue
		}
		var evidence holdingLinkedEvidence
		label := l.Text(i18n.HoldingReconViewActivity)
		decoded := jsoncodec.Unmarshal(link.Evidence, &evidence) == nil
		if decoded {
			if c.zone != nil && !evidence.Committed.EffectiveAt.IsZero() {
				label = fmt.Sprintf(l.Text(i18n.HoldingReconViewDatedActivity),
					historyDay(view, evidence.Committed.EffectiveAt.In(c.zone).Format(time.DateOnly)))
			}
			if evidence.Committed.Note != nil && *evidence.Committed.Note != "" {
				label += " · " + *evidence.Committed.Note
			}
		}
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
				rows = append(rows, holdingLinkedEvidenceRows(view, evidence, c.zone)...)
			}
		}
	}
	return rows
}
