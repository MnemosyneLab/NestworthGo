package desktop

import (
	"fmt"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type holdingProductDraft struct {
	id         domain.ProductContractID
	kind       domain.ProductKind
	link       func() *domain.ReconciliationLink
	name       string
	currency   string
	current    *domain.Money
	interest   *holdingProductInterest
	settlement *holdingProductSettlement
	terms      *holdingProductTerms
	renewal    *holdingProductRenewal
	add        *holdingProductAdd

	loading    <-chan historyResult[application.ProductDetail]
	loadFailed bool
	retry      widget.Clickable
}

type holdingProductAction uint8

const (
	holdingEditProductTerms holdingProductAction = iota
	holdingObserveProductValue
	holdingReceiveProductInterest
	holdingSettleProduct
	holdingRenewProduct
)

func managedProductCheckpoint(item domain.ReconciliationItem) (domain.ProductContract, bool) {
	var contract domain.ProductContract
	ok := item.Kind == "product" && item.ProductID != "" && !item.ArchivedNonzero &&
		jsoncodec.Unmarshal(item.Detail, &contract) == nil && (contract.Kind == domain.ProductLockedProduct || contract.Kind == domain.ProductTermDeposit)
	return contract, ok
}

func (p *tasksPage) openHoldingProductCorrection(account domain.ReconciliationAccount, item domain.ReconciliationItem, contract domain.ProductContract, entry *widget.Clickable, action holdingProductAction) {
	id, err := domain.ParseProductContractID(item.ProductID)
	if err != nil {
		return
	}
	c := p.holdingRecon
	d := newHoldingReconDraft(entry)
	d.accountID, d.itemID = account.AccountID, item.ID
	d.product = &holdingProductDraft{id: id, kind: contract.Kind, link: c.datedCheckpointLink(account.AccountID, item),
		name: account.Name + " · " + item.Label, currency: item.Currency}
	switch action {
	case holdingEditProductTerms:
		d.product.terms = newHoldingProductTerms()
	case holdingReceiveProductInterest:
		d.product.interest = &holdingProductInterest{remaining: ui.NewTextField(true)}
	case holdingSettleProduct:
		settlement := newHoldingProductSettlement(contract.Kind)
		d.product.settlement = &settlement
	case holdingRenewProduct:
		d.product.renewal = newHoldingProductRenewal(contract.Kind)
	}
	c.draft, c.saved = d, i18n.NoMessage
	p.loadHoldingProduct(d.product)
}

func (p *tasksPage) loadHoldingProduct(f *holdingProductDraft) {
	f.loadFailed = false
	f.loading = runHistoryRead(&p.workers, p.invalidate, func() (application.ProductDetail, error) {
		return p.holdingRecon.ledger.Product(p.ctx, f.id)
	})
}

func (p *tasksPage) previewHoldingProduct(d *holdingReconDraft) {
	f := d.product
	link := f.link()
	if link == nil {
		d.problem = i18n.HoldingReconChanged
		return
	}
	if f.terms != nil {
		terms, policy, ok := f.terms.inputs()
		if !ok {
			d.problem = i18n.ProductEditValidDays
			return
		}
		req := liquidity.UpdateProductTermsRequest{ProductID: f.id.String(), ExpectedRevision: f.terms.current.Revision,
			ReconciliationLink: link, Terms: terms, Policy: policy}
		d.reading = runHistoryRead(&p.workers, p.invalidate, func() (holdingReconReview, error) {
			review, err := p.holdingRecon.service.PreviewProductTerms(p.ctx, req)
			return holdingReconReview{planID: review.PlanID, terms: &review}, err
		})
		return
	}
	amount := strings.TrimSpace(d.total.Text())
	req, valid := f.operationRequest(*link, amount)
	if !valid {
		d.problem = i18n.ProductEditValidDays
		return
	}
	if req.Kind != "" {
		d.reading = runHistoryRead(&p.workers, p.invalidate, func() (holdingReconReview, error) {
			review, err := p.holdingRecon.service.PreviewProductOperation(p.ctx, req)
			return holdingReconReview{planID: review.PlanID, operation: review.Operation}, err
		})
		return
	}
	d.reading = runHistoryRead(&p.workers, p.invalidate, func() (holdingReconReview, error) {
		// Wails asks the shared service to capture the observation time at review.
		review, err := p.holdingRecon.service.PreviewProductValuation(p.ctx, *link, f.id.String(), amount, "")
		return holdingReconReview{planID: review.PlanID, valuation: review.Valuation}, err
	})
}

func (f *holdingProductDraft) operationRequest(link domain.ReconciliationLink, amount string) (liquidity.ProductCommandRequest, bool) {
	switch {
	case f.interest != nil:
		return f.interest.request(link, f.id, amount), true
	case f.settlement != nil:
		return f.settlement.request(link, f.id, amount), true
	case f.renewal != nil:
		return f.renewal.request(link, f.id, amount)
	case f.add != nil:
		return f.add.request(link, f.currency)
	default:
		return liquidity.ProductCommandRequest{}, true
	}
}

func (f *holdingProductDraft) title() i18n.MessageID {
	if f.add != nil {
		return i18n.ProductAddAction
	}
	if f.renewal != nil {
		return i18n.ProductRenewAction
	}
	if f.terms != nil {
		return i18n.ProductEditTerms
	}
	if f.interest != nil {
		return i18n.HoldingReconProductInterest
	}
	if f.settlement != nil {
		return i18n.ProductReceiptAction
	}
	return i18n.HoldingReconProductValuation
}

func (p *tasksPage) holdingProductFields(view ui.View, d *holdingReconDraft, ready bool) []layout.Widget {
	f, l := d.product, view.Language
	rows := []layout.Widget{view.Message(f.name, view.Colors.Ink)}
	switch {
	case f.loadFailed:
		rows = append(rows, view.Message(l.Text(i18n.ProductReadFailed), view.Colors.ErrorInk),
			taskLink(view, &f.retry, l.Text(i18n.HistoryRetry), func() { p.loadHoldingProduct(f) }))
	case f.loading != nil:
		rows = append(rows, view.Message(l.Text(i18n.ProductLoading), view.Colors.SecondaryInk))
	default:
		rows = append(rows, fundsAmount(view, i18n.ProductValue, f.current))
	}
	if f.terms != nil {
		return rows
	}
	enabled := ready && f.loading == nil && !f.loadFailed
	amountLabel := i18n.HoldingReconProductValuation
	if f.interest != nil {
		amountLabel = i18n.ProductActualInterest
	}
	if f.settlement != nil || f.renewal != nil {
		amountLabel = i18n.HoldingReconGrossProceeds
		if f.kind == domain.ProductTermDeposit {
			amountLabel = i18n.ProductReturnedPrincipal
		}
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		if enabled {
			d.focusAdjustmentField(gtx, &d.total)
		}
		return d.total.LayoutCurrency(gtx, view, ui.FieldOptions{Label: amountLabel, Disabled: !enabled}, f.currency)
	})
	return rows
}

func (p *tasksPage) holdingProductValuationReviewRows(view ui.View, review *liquidity.ProductValuationPreviewDTO) []layout.Widget {
	c, l := p.holdingRecon, view.Language
	value := func(label i18n.MessageID, m *wire.MoneyView) layout.Widget {
		text := l.Text(i18n.OverviewNoValue)
		if m != nil {
			text = holdingSnapshotValue(view, m.Amount, m.Currency)
		}
		return view.Message(fmt.Sprintf(l.Text(label), text), view.Colors.Ink)
	}
	rows := []layout.Widget{view.Message(c.draft.product.name, view.Colors.Ink),
		value(i18n.ProductValueBefore, review.CurrentValueBefore), value(i18n.ProductValueAfter, review.CurrentValueAfter)}
	if review.NetWorthKnown && review.NetWorthDelta != nil {
		delta := review.NetWorthDelta
		text := fmt.Sprintf(l.Text(i18n.HoldingReconValuationImpact), holdingSnapshotValue(view, delta.Amount, delta.Currency))
		rows = append(rows, view.Message(text, view.Colors.SecondaryInk))
	} else {
		text := fmt.Sprintf(l.Text(i18n.HoldingReconValuationImpact), l.Text(i18n.OverviewNoValue))
		rows = append(rows, view.Message(text, view.Colors.SecondaryInk))
	}
	if at, err := time.Parse(time.RFC3339Nano, review.Command.ObservedAt); err == nil {
		zone := c.zone
		if zone == nil {
			zone = time.UTC
		}
		rows = append(rows, view.Message(tradeTimestamp(view, at, zone), view.Colors.SecondaryInk))
	}
	return rows
}
