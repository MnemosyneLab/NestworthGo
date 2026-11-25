package desktop

import (
	"fmt"
	"slices"
	"time"

	"gioui.org/io/event"

	"gioui.org/layout"
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

func (p *tasksPage) startHoldingDatedChange(account domain.ReconciliationAccount, item domain.ReconciliationItem, entry *widget.Clickable) {
	c := p.holdingRecon
	at := slices.IndexFunc(c.directory.accounts, func(a domain.Account) bool { return a.ID.String() == account.AccountID })
	id, err := domain.ParseHoldingID(item.HoldingID)
	if at < 0 || err != nil {
		return
	}
	c.saved = i18n.NoMessage
	c.dated = &accountInvestment{service: c.ledger, onPosted: func(domain.Activity, i18n.MessageID, event.Tag) {
		c.saved, c.restore = i18n.HoldingReconDatedSaved, entry
		p.readHoldingReconciliation()
		p.read()
	}}
	c.dated.startAdjustment(p.holdingChangeRuntime(), positionScope{
		account: c.directory.accounts[at], holding: domain.Holding{ID: id}, action: entry,
	})
	c.dated.draft.adjustment.link = func() *domain.ReconciliationLink {
		a, current, found := c.checkpoint(account.AccountID, item.ID, true)
		if !found || c.session.Status != "in_progress" || current.HoldingID != id.String() ||
			current.ArchivedNonzero || current.ProductID != "" {
			return nil
		}
		return &domain.ReconciliationLink{SessionID: c.id, AccountID: a.AccountID, ItemID: current.ID,
			ExpectedRevision: c.session.Revision, FinancialRevision: a.FinancialRevision}
	}
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
		var evidence struct {
			Committed struct {
				EffectiveAt time.Time `json:"effectiveAt"`
				Note        *string   `json:"note"`
			} `json:"committed"`
		}
		label := l.Text(i18n.HoldingReconViewActivity)
		if jsoncodec.Unmarshal(link.Evidence, &evidence) == nil {
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
	}
	return rows
}
