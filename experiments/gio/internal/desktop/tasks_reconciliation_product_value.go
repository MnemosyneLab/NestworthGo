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

type holdingProductValuationDraft struct {
	id       domain.ProductContractID
	link     func() *domain.ReconciliationLink
	name     string
	currency string
	current  *domain.Money

	loading    <-chan historyResult[application.ProductDetail]
	loadFailed bool
	retry      widget.Clickable
}

func canReconcileProductValuation(item domain.ReconciliationItem) bool {
	var contract domain.ProductContract
	return item.Kind == "product" && item.ProductID != "" && !item.ArchivedNonzero &&
		jsoncodec.Unmarshal(item.Detail, &contract) == nil &&
		contract.Kind == domain.ProductLockedProduct && contract.State == domain.ProductStateOpen
}

func (p *tasksPage) openHoldingProductValuation(account domain.ReconciliationAccount, item domain.ReconciliationItem, entry *widget.Clickable) {
	id, err := domain.ParseProductContractID(item.ProductID)
	if err != nil {
		return
	}
	c := p.holdingRecon
	d := newHoldingReconDraft(entry)
	d.accountID, d.itemID = account.AccountID, item.ID
	d.valuation = &holdingProductValuationDraft{id: id, link: c.datedCheckpointLink(account.AccountID, item),
		name: account.Name + " · " + item.Label, currency: item.Currency}
	c.draft, c.saved = d, i18n.NoMessage
	p.loadHoldingProductValuation(d.valuation)
}

func (p *tasksPage) loadHoldingProductValuation(f *holdingProductValuationDraft) {
	f.loadFailed = false
	f.loading = runHistoryRead(&p.workers, p.invalidate, func() (application.ProductDetail, error) {
		return p.holdingRecon.ledger.Product(p.ctx, f.id)
	})
}

func (p *tasksPage) previewHoldingProductValuation(d *holdingReconDraft) {
	f := d.valuation
	link := f.link()
	if link == nil {
		d.problem = i18n.HoldingReconChanged
		return
	}
	amount := strings.TrimSpace(d.total.Text())
	d.reading = runHistoryRead(&p.workers, p.invalidate, func() (holdingReconReview, error) {
		// Wails asks the shared service to capture the observation time at review.
		review, err := p.holdingRecon.service.PreviewProductValuation(p.ctx, *link, f.id.String(), amount, "")
		return holdingReconReview{planID: review.PlanID, valuation: review.Valuation}, err
	})
}

func (p *tasksPage) holdingProductValuationFields(view ui.View, d *holdingReconDraft, ready bool) []layout.Widget {
	f, l := d.valuation, view.Language
	rows := []layout.Widget{view.Message(f.name, view.Colors.Ink)}
	switch {
	case f.loadFailed:
		rows = append(rows, view.Message(l.Text(i18n.ProductReadFailed), view.Colors.ErrorInk),
			taskLink(view, &f.retry, l.Text(i18n.HistoryRetry), func() { p.loadHoldingProductValuation(f) }))
	case f.loading != nil:
		rows = append(rows, view.Message(l.Text(i18n.ProductLoading), view.Colors.SecondaryInk))
	default:
		rows = append(rows, fundsAmount(view, i18n.ProductValue, f.current))
	}
	rows = append(rows, func(gtx layout.Context) layout.Dimensions {
		enabled := ready && f.loading == nil && !f.loadFailed
		if enabled {
			d.focusAdjustmentField(gtx, &d.total)
		}
		return d.total.LayoutCurrency(gtx, view, ui.FieldOptions{Label: i18n.HoldingReconProductValuation, Disabled: !enabled}, f.currency)
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
	rows := []layout.Widget{view.Message(c.draft.valuation.name, view.Colors.Ink),
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
