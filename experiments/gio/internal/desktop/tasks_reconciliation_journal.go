package desktop

import (
	"context"
	"fmt"
	"slices"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

const reconciliationJournalPageSize = 25

type reconciliationJournalVisit struct {
	open bool
	page int
}

type reconciliationJournal struct {
	reconciliationJournalVisit
	toggle, previous, next, retry widget.Clickable
	focus                         *widget.Clickable
}

// The session owns the receipt map. Account plus operation is the stored
// identity within the session, independent of page and visual position.
type reconciliationReceiptKey struct {
	account, operation string
}

type checkpointReceiptGroup struct {
	account, item string
}

func groupReconciliationActivityLinks(links []domain.ReconciliationLinkedChange) map[checkpointReceiptGroup][]domain.ReconciliationLinkedChange {
	groups := make(map[checkpointReceiptGroup][]domain.ReconciliationLinkedChange)
	for _, link := range links {
		if link.Family == "activity" {
			key := checkpointReceiptGroup{link.AccountID, link.ItemID}
			groups[key] = append(groups[key], link)
		}
	}
	return groups
}

func journalReceiptKey(link domain.ReconciliationLinkedChange) reconciliationReceiptKey {
	return reconciliationReceiptKey{link.AccountID, link.OperationID}
}

func (c *holdingReconciliation) journalReceiptButton(id reconciliationReceiptKey) *widget.Clickable {
	if c.journalReceipts[id] == nil {
		c.journalReceipts[id] = new(widget.Clickable)
	}
	return c.journalReceipts[id]
}

func (c *holdingReconciliation) restoreJournalReceipt() {
	if c.linksFailed || c.journalReturn.operation == "" {
		return
	}
	index := slices.IndexFunc(c.links, func(link domain.ReconciliationLinkedChange) bool {
		return journalReceiptKey(link) == c.journalReturn
	})
	if index >= 0 {
		c.linkedJournal.open = true
		c.linkedJournal.page = index / reconciliationJournalPageSize
	}
	c.journalReturn = reconciliationReceiptKey{}
}

func reconciliationAccountRoles(ctx context.Context, ledger reconciliationActivityLedger) map[string]domain.BalanceSheetRole {
	accounts, err := ledger.ListAccounts(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil
	}
	roles := make(map[string]domain.BalanceSheetRole, len(accounts))
	for _, a := range accounts {
		roles[a.Account.ID.String()] = a.Account.BalanceSheetRole
	}
	return roles
}

func (j *reconciliationJournal) bounds(total int) (int, int) {
	last := max(0, (total-1)/reconciliationJournalPageSize)
	j.page = min(j.page, last)
	start := j.page * reconciliationJournalPageSize
	return start, min(start+reconciliationJournalPageSize, total)
}

func (j *reconciliationJournal) pager(view ui.View, total int) layout.Widget {
	l := view.Language
	return func(gtx layout.Context) layout.Dimensions {
		first, end := j.bounds(total)
		if j.focus != nil {
			gtx.Execute(key.FocusCmd{Tag: j.focus})
			j.focus = nil
		}
		switch {
		case first > 0 && j.previous.Clicked(gtx):
			j.page--
			j.focus = &j.previous
			if j.page == 0 {
				j.focus = &j.next
			}
			gtx.Execute(op.InvalidateCmd{})
		case end < total && j.next.Clicked(gtx):
			j.page++
			j.focus = &j.next
			if _, last := j.bounds(total); last == total {
				j.focus = &j.previous
			}
			gtx.Execute(op.InvalidateCmd{})
		}
		label := view.Message(fmt.Sprintf(l.Text(i18n.HoldingReconJournalRange), first+1, end, total), view.Colors.SecondaryInk)
		controls := []layout.Widget{
			taskPagerButton(view, &j.previous, i18n.TasksPrevious, first > 0),
			taskPagerButton(view, &j.next, i18n.TasksNext, end < total),
			label,
		}
		return (ui.Wrap{Gap: 8}).Layout(gtx, len(controls), func(gtx layout.Context, i int) layout.Dimensions { return controls[i](gtx) })
	}
}

func (p *tasksPage) reconciliationJournalRows(view ui.View) []layout.Widget {
	c := p.holdingRecon
	l := view.Language
	rows := []layout.Widget{
		c.journalToggle(view, &c.recentJournal, l.Text(i18n.HoldingReconRecentJournal)),
	}
	if c.recentJournal.open {
		rows = append(rows, p.recentReconciliationRows(view)...)
	}
	rows = append(rows, c.journalToggle(view, &c.linkedJournal, l.Text(i18n.HoldingReconLinkedJournal)))
	if c.linkedJournal.open {
		rows = append(rows, p.linkedReconciliationRows(view)...)
	}
	return rows
}

func (c *holdingReconciliation) journalToggle(view ui.View, j *reconciliationJournal, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		c.restoreFocus(gtx, &j.toggle)
		return taskLink(view, &j.toggle, label, func() {
			j.open = !j.open
			gtx.Execute(key.FocusCmd{Tag: &j.toggle})
		})(gtx)
	}
}

func (p *tasksPage) retryReconciliationJournal(view ui.View, j *reconciliationJournal) layout.Widget {
	c := p.holdingRecon
	return taskLink(view, &j.retry, view.Language.Text(i18n.HistoryRetry), func() {
		c.editScroll, c.restoreEditScroll = view.PageScrollOffset(), true
		c.restore = &j.toggle
		p.readHoldingReconciliation()
	})
}

type reconciliationRecentEvidence struct {
	AccountName string `json:"accountName"`
	ProductName string `json:"productName"`
	Amount      string `json:"amount"`
	Currency    string `json:"currency"`
	Quantity    string `json:"quantity"`
}

func (p *tasksPage) recentReconciliationRows(view ui.View) []layout.Widget {
	c, l := p.holdingRecon, view.Language
	var rows []layout.Widget
	for _, a := range c.session.Accounts {
		if a.PriorSuccessfulAt == "" {
			baseline := fmt.Sprintf(l.Text(i18n.HoldingReconFirstBaseline), a.Name)
			rows = append(rows, view.Message(baseline, view.Colors.SecondaryInk))
		}
	}
	switch {
	case c.changesFailed:
		return append(rows, view.Message(l.Text(i18n.HoldingReconJournalReadFailed), view.Colors.ErrorInk), p.retryReconciliationJournal(view, &c.recentJournal))
	case len(c.changes) == 0:
		return append(rows, view.Message(l.Text(i18n.HoldingReconRecentEmpty), view.Colors.SecondaryInk))
	}
	start, end := c.recentJournal.bounds(len(c.changes))
	rows = append(rows, c.recentJournal.pager(view, len(c.changes)))
	for _, change := range c.changes[start:end] {
		var evidence reconciliationRecentEvidence
		_ = jsoncodec.Unmarshal(change.Evidence, &evidence)
		name := c.reconciliationAccountName(change.AccountID, evidence.AccountName)
		label := i18n.HoldingReconOriginalInterval
		if change.AfterCompletion {
			label = i18n.HoldingReconAfterCompletion
		}
		facts := []layout.Widget{view.Message(l.Text(label), view.Colors.SecondaryInk)}
		if evidence.ProductName != "" {
			facts = append(facts, view.Message(evidence.ProductName, view.Colors.Ink))
		}
		if evidence.Amount != "" {
			facts = append(facts, view.Message(holdingSnapshotValue(view, evidence.Amount, evidence.Currency), view.Colors.Ink))
		} else if evidence.Quantity != "" {
			facts = append(facts, view.Message(holdingSnapshotValue(view, evidence.Quantity, ""), view.Colors.Ink))
		}
		if at, err := time.Parse(time.RFC3339Nano, change.RecordedAt); err == nil {
			recorded := fmt.Sprintf(l.Text(i18n.HoldingReconRecordedAt), holdingEvidenceTime(view, at, c.zone))
			facts = append(facts, view.Message(recorded, view.Colors.SecondaryInk))
		}
		if at, err := time.Parse(time.RFC3339Nano, change.EffectiveAt); err == nil {
			zone := c.zone
			if zone == nil {
				zone = time.UTC
			}
			facts = append(facts, view.Message(tradeTimestamp(view, at, zone), view.Colors.SecondaryInk))
		}
		rows = append(rows, fundsCard(view, name, facts...))
	}
	return rows
}

func (c *holdingReconciliation) reconciliationAccountName(id, stored string) string {
	if stored != "" {
		return stored
	}
	for _, account := range c.session.Accounts {
		if account.AccountID == id {
			return account.Name
		}
	}
	return ""
}

func (p *tasksPage) linkedReconciliationRows(view ui.View) []layout.Widget {
	c, l := p.holdingRecon, view.Language
	switch {
	case c.linksFailed:
		return []layout.Widget{view.Message(l.Text(i18n.HoldingReconJournalReadFailed), view.Colors.ErrorInk), p.retryReconciliationJournal(view, &c.linkedJournal)}
	case len(c.links) == 0:
		return []layout.Widget{view.Message(l.Text(i18n.HoldingReconLinkedEmpty), view.Colors.SecondaryInk)}
	}
	start, end := c.linkedJournal.bounds(len(c.links))
	rows := []layout.Widget{c.linkedJournal.pager(view, len(c.links))}
	for index, link := range c.links[start:end] {
		title := fmt.Sprintf(l.Text(i18n.HoldingReconJournalEntry), start+index+1)
		var evidence holdingLinkedEvidence
		var facts []layout.Widget
		if jsoncodec.Unmarshal(link.Evidence, &evidence) == nil && evidence.Before != nil {
			before := evidence.Before
			if name := c.reconciliationAccountName(link.AccountID, before.AccountName); name != "" {
				title += " · " + name
			}
			if label := holdingCheckLabel(view, *before); label != "" {
				title += " · " + label
			}
			facts = holdingLinkedEvidenceRows(view, evidence, c.zone, c.accountRoles)
		} else {
			facts = []layout.Widget{view.Message(l.Text(i18n.OverviewNoValue), view.Colors.SecondaryInk)}
		}
		if p.openActivity != nil && link.Family == "activity" {
			if id, err := domain.ParseActivityID(link.OperationID); err == nil {
				button := c.journalReceiptButton(journalReceiptKey(link))
				label := holdingLinkedReceiptLabel(view, c.zone, evidence)
				facts = append(facts, taskLink(view, button, label, func() { p.openActivity(id, button) }))
			}
		}
		rows = append(rows, fundsCard(view, title, facts...))
	}
	return rows
}
