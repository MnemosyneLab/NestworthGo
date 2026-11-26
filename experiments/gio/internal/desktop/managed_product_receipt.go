package desktop

import (
	"context"
	"errors"
	"slices"
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
	return slices.Contains(r.detail.PermittedActions, string(domain.ProductOpSettle))
}

func (r productRead) canRecordReceipt() bool {
	return r.detail != nil && r.detail.Contract.State == domain.ProductStateOpen &&
		slices.Contains(r.detail.PermittedActions, string(domain.ProductOpSettle))
}

func (r productRead) canRecordInterest() bool {
	return r.detail != nil && slices.Contains(r.detail.PermittedActions, string(domain.ProductOpReceiveInterest))
}

type productReceiptInput struct {
	principal, interest, fee string
	day, minute              string
	fromTasks                bool
}

type productInterestInput struct {
	amount, remaining, paidThrough string
	day, minute                    string
}

func reviewProductInterest(ctx context.Context, service productReceiptLedger, id domain.ProductContractID, input productInterestInput, clock func() time.Time) (plan productReceiptPlan, err error) {
	err = service.WithRead(ctx, func(ctx context.Context) error {
		detail, err := service.Product(ctx, id)
		if err != nil {
			return err
		}
		if !slices.Contains(detail.PermittedActions, string(domain.ProductOpReceiveInterest)) {
			return errProductReceiptUnavailable
		}
		interest := &application.ReceiveInterestCommand{ProductID: id.String(), Amount: input.amount,
			EffectiveLocalDate: input.day, EffectiveLocalTime: input.minute,
			InterestPaidThroughOn: productOptional(input.paidThrough), RemainingInterest: productOptional(input.remaining)}
		if input.day == "" && input.minute == "" {
			interest.EffectiveAt = clock().UTC().Format(time.RFC3339Nano)
		}
		plan.command = application.ProductCommand{Kind: domain.ProductOpReceiveInterest, ReceiveInterest: interest}
		plan.review, err = service.PreviewProductOperation(ctx, plan.command)
		if err != nil {
			return err
		}
		plan.mutation = domain.NewProductOperationID()
		return nil
	})
	return plan, err
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
		if !current.canRecordReceipt() || input.fromTasks && !current.canConfirmReceipt() {
			return errProductReceiptUnavailable
		}
		contract := current.detail.Contract
		deposit := contract.Kind == domain.ProductTermDeposit
		if input.fromTasks && input.day != "" && input.day < *contract.MaturityOn {
			return &domain.Error{Code: domain.ErrInvalidChangeTime}
		}
		// Pass the actual amounts through unchanged. An explicit timestamp
		// gives the blank "now" option the same effective time on review,
		// confirmation, and receipt replay.
		settle := &application.SettleProductCommand{
			ProductID: id.String(), Fee: &input.fee,
			EffectiveLocalDate: input.day, EffectiveLocalTime: input.minute,
		}
		if deposit {
			settle.ReturnedPrincipal, settle.Interest = &input.principal, &input.interest
		} else {
			settle.GrossProceeds = &input.principal
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
		if input.fromTasks {
			day, _, err := domain.LocalCivilDate(plan.review.EffectiveAt, plan.review.Timezone)
			if err != nil {
				return err
			}
			if day < *contract.MaturityOn {
				return &domain.Error{Code: domain.ErrInvalidChangeTime}
			}
		}
		plan.mutation = domain.NewProductOperationID()
		return nil
	})
	return plan, err
}

type productReceiptDraft struct {
	name, currency, earliest, timezone   string
	kind                                 domain.ProductKind
	fromTasks                            bool
	principal, interest, fee             ui.TextField
	date                                 ui.DatePicker
	time                                 ui.TimePicker
	day, minute                          string
	preview, edit, confirm               widget.Clickable
	replay, cancel                       widget.Clickable
	reviewList                           widget.List
	reviewing                            <-chan historyResult[productReceiptPlan]
	posting                              <-chan historyResult[application.ProductOperationReceipt]
	plan                                 *productReceiptPlan
	problem                              i18n.MessageID
	uncertain, focusEditor, focusConfirm bool
	received                             *productInterestFields
}

type productInterestFields struct {
	mode                      domain.InterestMode
	minimum, maturity, paidOn string
	paidThrough               ui.DatePicker
	remaining                 ui.TextField
}

func (f *productInterestFields) receive(contract domain.ProductContract) {
	f.mode, f.minimum, f.maturity = contract.InterestMode, contract.StartOn, productString(contract.MaturityOn)
	if contract.InterestPaidThroughOn != nil {
		f.minimum = *contract.InterestPaidThroughOn
	}
}

func (p *productDetailPage) startInterest() {
	detail := p.current.detail
	contract := detail.Contract
	fields := &productInterestFields{remaining: ui.NewTextField(true)}
	fields.receive(contract)
	p.saved = i18n.NoMessage
	p.receipt = &productReceiptDraft{name: productName(*detail), kind: contract.Kind, focusEditor: true,
		currency: contract.Currency.String(), timezone: p.current.timezone, principal: ui.NewTextField(true),
		reviewList: widget.List{List: layout.List{Axis: layout.Vertical}},
		received:   fields}
}

func (p *productDetailPage) receiptAction() *widget.Clickable {
	if p.receipt.received != nil {
		return &p.startInterestReceipt
	}
	return &p.startConfirmation
}

func (p *productDetailPage) startReceipt(fromTasks bool) {
	contract := p.current.detail.Contract
	d := &productReceiptDraft{
		name: productName(*p.current.detail), kind: contract.Kind, focusEditor: true,
		fromTasks:  fromTasks,
		reviewList: widget.List{List: layout.List{Axis: layout.Vertical}},
		currency:   contract.Currency.String(), timezone: p.current.timezone,
		principal: ui.NewTextField(true), interest: ui.NewTextField(true), fee: ui.NewTextField(true),
	}
	if fromTasks {
		d.earliest = productString(contract.MaturityOn)
	}
	if contract.Kind == domain.ProductTermDeposit {
		d.principal.Editor.SetText(contract.Principal.CanonicalAmount())
		d.interest.Editor.SetText("0")
	}
	d.fee.Editor.SetText("0")
	p.saved = i18n.NoMessage
	p.receipt = d
}

func (p *productDetailPage) reviewReceipt() {
	d := p.receipt
	if d.reviewing != nil || strings.TrimSpace(d.principal.Text()) == "" {
		return
	}
	d.problem = i18n.NoMessage
	id := p.id
	if d.received != nil {
		input := productInterestInput{amount: strings.TrimSpace(d.principal.Text()), day: d.day, minute: d.minute}
		switch d.received.mode {
		case domain.InterestManualMaturityAmount:
			input.remaining = strings.TrimSpace(d.received.remaining.Text())
		case domain.InterestSimpleAct365, domain.InterestSimpleAct360:
			input.paidThrough = d.received.paidOn
		}
		d.reviewing = runHistoryRead(&p.workers, p.invalidate, func() (productReceiptPlan, error) {
			return reviewProductInterest(p.ctx, p.receipts, id, input, p.clock)
		})
		return
	}
	input := productReceiptInput{principal: strings.TrimSpace(d.principal.Text()), interest: strings.TrimSpace(d.interest.Text()),
		fee: strings.TrimSpace(d.fee.Text()), day: d.day, minute: d.minute, fromTasks: d.fromTasks}
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
			d.plan, d.focusConfirm = &result.value, true
			d.reviewList.Position = layout.Position{}
		}
	default:
	}
	select {
	case result := <-d.posting:
		d.posting = nil
		if result.err == nil && result.value.OperationID == d.plan.mutation && len(result.value.ActivityIDs) != 0 {
			message := i18n.ProductReceiptSaved
			if d.received != nil {
				message = i18n.ProductStandaloneInterestSaved
			}
			p.receipt = nil
			p.refreshAfterProductWrite(message)
			return
		}
		problem := productReceiptProblem(result.err)
		switch {
		case problem == i18n.ProductReceiptChanged || problem == i18n.ProductReceiptTimeInvalid || problem == i18n.ProductReceiptStale,
			d.received != nil && problem == i18n.ProductReceiptAmountsInvalid:
			// Receipt lookup precedes validation in the shared service.
			// These rejections establish that this plan did not post.
			d.problem = productReceiptProblem(result.err)
			d.plan, d.uncertain, d.focusEditor = nil, false, true
			p.read()
		default:
			d.uncertain, d.focusConfirm = true, true
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
	case domain.ErrValidation, domain.ErrDecimalOverflow, domain.ErrUnsupportedPartialOperation:
		switch issue.Field {
		case "effectiveAt", "effectiveLocalDate", "effectiveLocalTime", "effectiveLocalDateTime":
			return i18n.ProductReceiptTimeInvalid
		}
		return i18n.ProductReceiptAmountsInvalid
	}
	return i18n.ProductReceiptPreviewFailed
}
