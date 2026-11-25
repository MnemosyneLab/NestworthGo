package desktop

import (
	"context"
	"errors"
	"strings"

	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/statement"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
	"github.com/waltwang/nestworth-go/internal/wailsapi/reconciliation"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type holdingReconcileAPI interface {
	Get(context.Context, string) (domain.ReconciliationSession, error)
	Preview(context.Context, application.ReconciliationAction) (reconciliation.SessionPreview, error)
	Commit(context.Context, string) (domain.ReconciliationSession, error)
	PreviewAdjustment(context.Context, reconciliation.AdjustmentRequest) (reconciliation.AdjustmentPreview, error)
	CommitAdjustment(context.Context, string) ([]wire.ChangePreviewDTO, error)
}

type holdingReconDirectory struct {
	sessions []domain.ReconciliationSession
	accounts []domain.Account
}

type holdingReconciliation struct {
	service                     holdingReconcileAPI
	directory                   holdingReconDirectory
	start, back, reload, resume widget.Clickable
	sessionButtons              map[string]*widget.Clickable
	itemButtons                 map[string]*holdingReconItemActions
	restore, sessionEntry       *widget.Clickable
	id                          string
	session                     *domain.ReconciliationSession
	loading                     <-chan historyResult[domain.ReconciliationSession]
	readFailed                  bool
	saved                       i18n.MessageID
	draft                       *holdingReconDraft
}

type holdingReconDraft struct {
	// A holding item identifies an adjustment. Otherwise action is start or resume.
	itemID, accountID                    string
	action                               statement.Action
	entry                                *widget.Clickable
	accountPick                          ui.Select
	total, note                          ui.TextField
	target                               *holdingReconQuantity
	cancel, preview, edit, confirm       widget.Clickable
	reading                              <-chan historyResult[holdingReconReview]
	writing                              <-chan historyResult[string]
	review                               *holdingReconReview
	problem                              i18n.MessageID
	initialFocus, reviewFocus, uncertain bool
}

type holdingReconItemActions struct {
	cost, quantity widget.Clickable
}

type holdingReconQuantity struct {
	quantity, unitCost ui.TextField
}

type holdingReconReview struct {
	planID  string
	session *domain.ReconciliationSession
	changes []wire.ChangePreviewDTO
}

func (c *holdingReconciliation) active() bool {
	return c != nil && (c.id != "" || c.draft != nil)
}

func (c *holdingReconciliation) unresolved() bool {
	return c != nil && c.draft != nil && (c.draft.writing != nil || c.draft.uncertain)
}

func (c *holdingReconciliation) discard() {
	if c != nil {
		c.id, c.session, c.loading, c.draft = "", nil, nil, nil
		c.saved, c.readFailed, c.restore, c.sessionEntry = i18n.NoMessage, false, nil, nil
		c.itemButtons = nil
	}
}

func newHoldingReconDraft(entry *widget.Clickable) *holdingReconDraft {
	return &holdingReconDraft{entry: entry, total: ui.NewTextField(true), note: ui.NewTextField(true), initialFocus: true}
}

func (p *tasksPage) openHoldingReconciliation(id string, entry *widget.Clickable) {
	c := p.holdingRecon
	c.id, c.sessionEntry, c.saved, c.draft = id, entry, i18n.NoMessage, nil
	c.itemButtons = make(map[string]*holdingReconItemActions)
	c.restore = &c.back
	p.readHoldingReconciliation()
}

func (p *tasksPage) readHoldingReconciliation() {
	c := p.holdingRecon
	c.session, c.readFailed = nil, false
	id := c.id
	c.loading = runHistoryRead(&p.workers, p.invalidate, func() (domain.ReconciliationSession, error) {
		return c.service.Get(p.ctx, id)
	})
}

func (c *holdingReconciliation) item(d *holdingReconDraft) (domain.ReconciliationAccount, domain.ReconciliationItem, bool) {
	if c.session != nil {
		for _, a := range c.session.Accounts {
			if a.AccountID != d.accountID {
				continue
			}
			for _, item := range a.Items {
				if item.ID == d.itemID && item.Kind == "holding" && item.HoldingID != "" {
					return a, item, true
				}
			}
		}
	}
	return domain.ReconciliationAccount{}, domain.ReconciliationItem{}, false
}

func (c *holdingReconciliation) canReview(d *holdingReconDraft) bool {
	if d.itemID != "" {
		_, item, ok := c.item(d)
		return ok && c.session.Status == "in_progress" && item.ProductID == "" &&
			!item.ArchivedNonzero && d.targetReady()
	}
	if d.action == statement.Start {
		return d.accountID != ""
	}
	return c.session != nil && c.session.Status == "paused"
}

func (d *holdingReconDraft) targetReady() bool {
	if d.target != nil {
		return strings.TrimSpace(d.target.quantity.Text()) != ""
	}
	return strings.TrimSpace(d.total.Text()) != ""
}

func (p *tasksPage) previewHoldingReconciliation() {
	c, d := p.holdingRecon, p.holdingRecon.draft
	if d.reading != nil || c.loading != nil || c.readFailed {
		return
	}
	d.problem = i18n.NoMessage
	if d.itemID != "" {
		a, item, found := c.item(d)
		if !found {
			d.problem = i18n.HoldingReconChanged
			return
		}
		request := reconciliation.AdjustmentRequest{
			Link: domain.ReconciliationLink{SessionID: c.id, AccountID: a.AccountID, ItemID: item.ID,
				ExpectedRevision: c.session.Revision, FinancialRevision: a.FinancialRevision},
			HoldingID: item.HoldingID, Currency: item.Currency, TotalCost: strings.TrimSpace(d.total.Text()), Note: d.note.Text(),
		}
		// A cost-only request leaves quantity to the shared service. An explicit
		// target uses the checkpoint's currency and the reviewed revision.
		if d.target != nil {
			request.Quantity = strings.TrimSpace(d.target.quantity.Text())
			request.UnitCost = strings.TrimSpace(d.target.unitCost.Text())
		}
		d.reading = runHistoryRead(&p.workers, p.invalidate, func() (holdingReconReview, error) {
			preview, err := c.service.PreviewAdjustment(p.ctx, request)
			return holdingReconReview{planID: preview.PlanID, changes: preview.Changes}, err
		})
		return
	}
	action := application.ReconciliationAction{Action: string(d.action), Note: d.note.Text()}
	if d.action == statement.Start {
		if d.accountID == "" {
			return
		}
		action.AccountIDs = []string{d.accountID}
	} else if c.session != nil {
		action.SessionID, action.ExpectedRevision = c.id, c.session.Revision
	} else {
		return
	}
	d.reading = runHistoryRead(&p.workers, p.invalidate, func() (holdingReconReview, error) {
		preview, err := c.service.Preview(p.ctx, action)
		return holdingReconReview{planID: preview.PlanID, session: &preview.Result}, err
	})
}

func (p *tasksPage) confirmHoldingReconciliation() {
	c, d := p.holdingRecon, p.holdingRecon.draft
	if d.writing != nil || d.review == nil {
		return
	}
	id, cost, sessionID := d.review.planID, d.itemID != "", c.id
	d.writing = runHistoryRead(&p.workers, p.invalidate, func() (string, error) {
		if cost {
			results, err := c.service.CommitAdjustment(p.ctx, id)
			if err != nil || len(results) == 0 {
				return "", err
			}
			return sessionID, nil
		}
		session, err := c.service.Commit(p.ctx, id)
		return session.ID, err
	})
}

func (p *tasksPage) pollHoldingReconciliation() {
	c := p.holdingRecon
	select {
	case result := <-c.loading:
		c.loading = nil
		c.readFailed = result.err != nil
		if result.err == nil {
			c.session = &result.value
		}
	default:
	}
	d := c.draft
	if d == nil {
		return
	}
	select {
	case result := <-d.reading:
		d.reading = nil
		switch {
		case result.err == nil && result.value.planID != "":
			d.review, d.reviewFocus = &result.value, true
		case holdingReconStale(result.err) && c.id != "":
			d.problem = i18n.HoldingReconChanged
			p.readHoldingReconciliation()
		default:
			d.problem = holdingReconIssue(result.err, d.target != nil)
		}
	default:
	}
	select {
	case result := <-d.writing:
		d.writing = nil
		if result.err == nil && result.value != "" {
			if c.id != result.value {
				p.openHoldingReconciliation(result.value, d.entry)
			} else {
				c.draft, c.restore, c.saved = nil, d.entry, i18n.NoMessage
				if d.target != nil {
					c.saved = i18n.HoldingReconQuantitySaved
				} else if d.itemID != "" {
					c.saved = i18n.HoldingReconSaved
				}
				if d.action == statement.Resume {
					c.restore = &c.back
				}
				p.readHoldingReconciliation()
			}
			p.read()
			return
		}
		if holdingReconStale(result.err) {
			d.review, d.uncertain, d.problem, d.initialFocus = nil, false, i18n.HoldingReconChanged, true
			if c.id != "" {
				p.readHoldingReconciliation()
			}
		} else {
			d.uncertain, d.reviewFocus = true, true
		}
	default:
	}
}

func holdingReconStale(err error) bool {
	var issue *apierror.WireError
	if !errors.As(err, &issue) {
		return false
	}
	if issue.Code == string(domain.ErrStalePreview) {
		return true
	}
	if issue.Code == string(domain.ErrConflict) {
		switch issue.Field {
		case "financialRevision", "expectedRevision", "revision", "reconciliationLink", "preview":
			return true
		}
	}
	return false
}

func holdingReconIssue(err error, quantity bool) i18n.MessageID {
	var issue *apierror.WireError
	if errors.As(err, &issue) {
		if issue.Code == string(domain.ErrNoChange) {
			if quantity {
				return i18n.HoldingReconQuantityUnchanged
			}
			return i18n.HoldingReconUnchanged
		}
		field := issue.Field[strings.LastIndexByte(issue.Field, '.')+1:]
		switch field {
		case "totalCost":
			return i18n.HoldingReconInvalidTotal
		case "quantity", "targetQuantity":
			return i18n.HoldingReconQuantityInvalid
		case "unitCost":
			return i18n.HoldingReconAcquisitionCostInvalid
		}
	}
	return i18n.HoldingReconFailed
}
