package desktop

import (
	"context"
	"errors"
	"strings"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

var errProductReceiptUnavailable = &domain.Error{Code: domain.ErrConflict}

type productReceiptLedger interface {
	productReader
	PreviewProductOperation(context.Context, application.ProductCommand) (application.ProductOperationPreview, error)
	RecordProductOperation(context.Context, application.ProductCommand, string, string) (application.ProductOperationReceipt, error)
}

func (r productRead) canConfirmReceipt() bool {
	if r.detail == nil || r.detail.Contract.Kind != domain.ProductTermDeposit || r.detail.Contract.State != domain.ProductStateOpen ||
		r.detail.DisplayState != domain.ProductDisplayDueUnconfirmed || !r.accountActive ||
		r.detail.Contract.MaturityOn == nil || r.localDate < *r.detail.Contract.MaturityOn {
		return false
	}
	for _, action := range r.detail.PermittedActions {
		if action == "settle" {
			return true
		}
	}
	return false
}

type productReceiptInput struct {
	principal, interest, fee string
	day, minute              string
}

type productReceiptPlan struct {
	command  application.ProductCommand
	review   application.ProductOperationPreview
	mutation domain.ProductOperationID
}

func reviewProductReceipt(ctx context.Context, service productReceiptLedger, id domain.ProductContractID, input productReceiptInput, clock func() time.Time) (plan productReceiptPlan, err error) {
	err = service.WithRead(ctx, func(ctx context.Context) error {
		current, err := readProduct(ctx, service, id)
		if err != nil {
			return err
		}
		if !current.canConfirmReceipt() {
			return errProductReceiptUnavailable
		}
		if input.day != "" && input.day < *current.detail.Contract.MaturityOn {
			return &domain.Error{Code: domain.ErrInvalidChangeTime}
		}
		// Pass the actual amounts through unchanged. An explicit timestamp
		// gives the blank "now" option the same effective time on review,
		// confirmation, and receipt replay.
		settle := &application.SettleProductCommand{
			ProductID: id.String(), ReturnedPrincipal: &input.principal,
			Interest: &input.interest, Fee: &input.fee,
			EffectiveLocalDate: input.day, EffectiveLocalTime: input.minute,
		}
		if input.day == "" && input.minute == "" {
			settle.EffectiveAt = clock().UTC().Format(time.RFC3339Nano)
		}
		for _, reservation := range current.detail.Reservations {
			if reservation.Active() {
				settle.ReleaseReservationIDs = append(settle.ReleaseReservationIDs, reservation.ID.String())
			}
		}
		plan.command = application.ProductCommand{Kind: domain.ProductOpSettle, Settle: settle}
		plan.review, err = service.PreviewProductOperation(ctx, plan.command)
		if err != nil {
			return err
		}
		day, _, err := domain.LocalCivilDate(plan.review.EffectiveAt, plan.review.Timezone)
		if err != nil {
			return err
		}
		if day < *current.detail.Contract.MaturityOn {
			return &domain.Error{Code: domain.ErrInvalidChangeTime}
		}
		plan.mutation = domain.NewProductOperationID()
		return nil
	})
	return plan, err
}

type productReceiptDraft struct {
	name, currency, maturity, timezone string
	principal, interest, fee           ui.TextField
	date                               ui.DatePicker
	time                               ui.TimePicker
	day, minute                        string
	preview, edit, confirm             widget.Clickable
	replay, cancel                     widget.Clickable
	reviewList                         widget.List
	reviewing                          <-chan historyResult[productReceiptPlan]
	posting                            <-chan historyResult[application.ProductOperationReceipt]
	plan                               *productReceiptPlan
	problem                            i18n.MessageID
	uncertain, focusEditor             bool
}

func (p *productDetailPage) startReceipt() {
	contract := p.current.detail.Contract
	d := &productReceiptDraft{
		name: productName(*p.current.detail), maturity: *contract.MaturityOn, focusEditor: true,
		reviewList: widget.List{List: layout.List{Axis: layout.Vertical}},
		currency:   contract.Currency.String(), timezone: p.current.timezone,
		principal: ui.NewTextField(true), interest: ui.NewTextField(true), fee: ui.NewTextField(true),
	}
	d.principal.Editor.SetText(contract.Principal.CanonicalAmount())
	d.interest.Editor.SetText("0")
	d.fee.Editor.SetText("0")
	p.receipt = d
}

func (p *productDetailPage) reviewReceipt() {
	d := p.receipt
	if d.reviewing != nil || strings.TrimSpace(d.principal.Text()) == "" {
		return
	}
	input := productReceiptInput{strings.TrimSpace(d.principal.Text()), strings.TrimSpace(d.interest.Text()), strings.TrimSpace(d.fee.Text()), d.day, d.minute}
	d.problem = i18n.NoMessage
	id := p.id
	d.reviewing = runHistoryRead(&p.workers, p.invalidate, func() (productReceiptPlan, error) {
		return reviewProductReceipt(p.ctx, p.receipts, id, input, p.clock)
	})
}

func (p *productDetailPage) postReceipt() {
	d := p.receipt
	if d.posting != nil || d.plan == nil {
		return
	}
	plan := *d.plan
	d.posting = runHistoryRead(&p.workers, p.invalidate, func() (application.ProductOperationReceipt, error) {
		return p.receipts.RecordProductOperation(p.ctx, plan.command, plan.mutation.String(), plan.review.ReviewedStateHash)
	})
}

func (p *productDetailPage) receiptUnresolved() bool {
	return p != nil && p.receipt != nil && (p.receipt.posting != nil || p.receipt.uncertain)
}

func (p *productDetailPage) pollReceipt() {
	d := p.receipt
	if d == nil {
		return
	}
	select {
	case result := <-d.reviewing:
		d.reviewing = nil
		if result.err != nil {
			d.problem = productReceiptProblem(result.err)
			if d.problem == i18n.ProductReceiptChanged {
				p.read()
			}
		} else {
			d.plan = &result.value
			d.reviewList.Position = layout.Position{}
		}
	default:
	}
	select {
	case result := <-d.posting:
		d.posting = nil
		if result.err == nil && result.value.OperationID == d.plan.mutation && len(result.value.ActivityIDs) != 0 {
			p.receipt, p.receiptSaved = nil, true
			p.list.Position = layout.Position{}
			if p.onReceiptSaved != nil {
				p.onReceiptSaved()
			} else {
				p.read()
			}
			return
		}
		switch productReceiptProblem(result.err) {
		case i18n.ProductReceiptChanged, i18n.ProductReceiptTimeInvalid, i18n.ProductReceiptStale:
			// Receipt lookup precedes validation in the shared service.
			// These rejections establish that this plan did not post.
			d.problem = productReceiptProblem(result.err)
			d.plan, d.uncertain, d.focusEditor = nil, false, true
			p.read()
		default:
			d.uncertain = true
		}
	default:
	}
}

func productReceiptProblem(err error) i18n.MessageID {
	if errors.Is(err, errProductReceiptUnavailable) {
		return i18n.ProductReceiptChanged
	}
	var issue *domain.Error
	if !errors.As(err, &issue) {
		return i18n.ProductReceiptPreviewFailed
	}
	switch issue.Code {
	case domain.ErrStalePreview, domain.ErrUnresolvedReservationRelease:
		return i18n.ProductReceiptStale
	case domain.ErrConflict, domain.ErrNotFound:
		if issue.Field == "productId" {
			return i18n.ProductReceiptChanged
		}
	case domain.ErrInvalidChangeTime:
		return i18n.ProductReceiptTimeInvalid
	case domain.ErrValidation, domain.ErrDecimalOverflow:
		switch issue.Field {
		case "effectiveAt", "effectiveLocalDate", "effectiveLocalTime", "effectiveLocalDateTime":
			return i18n.ProductReceiptTimeInvalid
		}
		return i18n.ProductReceiptAmountsInvalid
	}
	return i18n.ProductReceiptPreviewFailed
}
