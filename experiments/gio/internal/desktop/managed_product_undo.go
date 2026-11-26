package desktop

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *productDetailPage) canStartGroupedUndo() bool {
	h := &p.history
	return p.receipts != nil && p.request == nil && h.pending == nil && h.problem == i18n.NoMessage &&
		h.undo.Allowed && h.undo.Operation != nil
}

func (p *productDetailPage) startUndo() {
	p.saved = i18n.NoMessage
	p.receipt = &productReceiptDraft{
		name: productName(*p.current.detail), currency: p.current.detail.Contract.Currency.String(),
		undo: p.history.undo.Operation, focusEditor: true,
		reviewList: widget.List{List: layout.List{Axis: layout.Vertical}},
	}
}

func reviewProductUndo(ctx context.Context, service productReceiptLedger, id domain.ProductContractID, target domain.ProductOperationID) (plan productReceiptPlan, err error) {
	err = service.WithRead(ctx, func(ctx context.Context) error {
		history, err := service.ListProductOperations(ctx, id, "", 1)
		if err != nil {
			return err
		}
		if !history.Undo.Allowed || history.Undo.Operation == nil || history.Undo.Operation.ID != target {
			return errProductReceiptUnavailable
		}
		plan.command = application.ProductCommand{Kind: domain.ProductOpUndo, Undo: &application.UndoProductCommand{OperationID: target.String()}}
		plan.review, err = service.PreviewProductOperation(ctx, plan.command)
		if err != nil {
			return err
		}
		plan.mutation = domain.NewProductOperationID()
		return nil
	})
	return plan, err
}

func productUndoProblem(err error) i18n.MessageID {
	var issue *domain.Error
	if !errors.As(err, &issue) {
		return i18n.NoMessage
	}
	switch issue.Code {
	case domain.ErrStalePreview, domain.ErrRevisionConflict, domain.ErrUnresolvedReservationRelease:
		return i18n.ProductUndoStale
	case domain.ErrUnsafeUndo, domain.ErrAlreadyUndone, domain.ErrInsufficientBalance, domain.ErrInsufficientQuantity:
		return i18n.ProductUndoUnavailable
	case domain.ErrConflict, domain.ErrNotFound:
		if issue.Field == "accountId" || issue.Field == "holdingId" || issue.Field == "operationId" {
			return i18n.ProductUndoUnavailable
		}
	}
	return i18n.NoMessage
}

func productUndoReason(reason application.ProductUndoDisabledReason) i18n.MessageID {
	switch reason {
	case application.UndoNoActiveOperation:
		return i18n.NoMessage
	case application.UndoValuationRequiresCorrection:
		return i18n.ProductUndoValuation
	case application.UndoLaterProductOperation:
		return i18n.ProductUndoLaterOperation
	case application.UndoContractChanged:
		return i18n.ProductUndoContract
	case application.UndoActiveReservation:
		return i18n.ProductUndoActiveReservation
	case application.UndoReservationChanged:
		return i18n.ProductUndoChangedReservation
	case application.UndoArchivedTarget:
		return i18n.ProductUndoArchived
	case application.UndoInsufficientBalance:
		return i18n.ProductUndoBalance
	case application.UndoInsufficientQuantity:
		return i18n.ProductUndoQuantity
	default:
		return i18n.ProductUndoUnavailable
	}
}

func (p *productDetailPage) layoutUndo(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.receipt, view.Language
	zone := p.history.zone
	if zone == nil {
		zone = time.UTC
	}
	target := fmt.Sprintf(l.Text(i18n.ProductUndoTarget), l.Text(productHistoryKind(d.undo.Kind)), tradeTimestamp(view, d.undo.EffectiveAt, zone))
	fresh := p.canStartGroupedUndo() && p.history.undo.Operation.ID == d.undo.ID
	if d.preview.Clicked(gtx) && fresh {
		p.reviewReceipt()
		gtx.Execute(op.InvalidateCmd{})
	}
	status := d.problem
	switch {
	case d.reviewing != nil || p.request != nil || p.history.pending != nil:
		status = i18n.HistoryLoading
	case !fresh:
		status = i18n.ProductUndoUnavailable
		if p.history.problem != i18n.NoMessage {
			status = p.history.problem
		} else if p.history.undo.Reason != nil {
			if reason := productUndoReason(*p.history.undo.Reason); reason != i18n.NoMessage {
				status = reason
			}
		}
	}
	rows := []ui.ColumnChild{
		ui.Rigid(tradeTitle(view, i18n.ProductGroupedUndo)),
		ui.Rigid(view.Message(target, view.Colors.Ink)),
		ui.Rigid(view.Message(l.Text(i18n.ProductUndoHelp), view.Colors.SecondaryInk)),
	}
	if status != i18n.NoMessage {
		color := view.Colors.ErrorInk
		if status == i18n.HistoryLoading {
			color = view.Colors.SecondaryInk
		}
		rows = append(rows, ui.Rigid(view.Message(l.Text(status), color)))
	}
	rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		if d.focusEditor && fresh {
			gtx.Execute(key.FocusCmd{Tag: &d.preview})
			d.focusEditor = false
		}
		actions := []layout.Widget{
			view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel)),
			func(gtx layout.Context) layout.Dimensions {
				if !fresh || d.reviewing != nil {
					gtx = gtx.Disabled()
				}
				return view.PrimaryButton(&d.preview, l.Text(i18n.ProductUndoPreview))(gtx)
			},
		}
		return (ui.Wrap{Gap: ui.Space8}).Layout(gtx, len(actions), func(gtx layout.Context, i int) layout.Dimensions {
			return actions[i](gtx)
		})
	}))
	return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
}

func productUndoReviewRows(view ui.View, review application.ProductOperationPreview) []layout.Widget {
	l := view.Language
	cashRow := func(label i18n.MessageID, amounts []domain.Money) layout.Widget {
		if len(amounts) == 0 {
			return fundsAmount(view, label, nil)
		}
		values := make([]string, 0, len(amounts))
		for _, amount := range amounts {
			values = append(values, portfolioDecimal(view, amount.CanonicalAmount())+" "+amount.Currency().String())
		}
		return labeledAmount(view, label, strings.Join(values, " · "), view.Colors.Ink)
	}
	delta := l.Text(i18n.OverviewNoValue)
	if review.NetWorthKnown && review.NetWorthDelta != nil {
		delta = portfolioDecimal(view, review.NetWorthDelta.CanonicalAmount()) + " " + review.NetWorthDelta.Currency().String()
	}
	rows := []layout.Widget{
		cashRow(i18n.ProductCashBefore, review.CashBefore),
		cashRow(i18n.ProductCashAfter, review.CashAfter),
		fundsAmount(view, i18n.ProductValueBefore, review.ProductBefore),
		fundsAmount(view, i18n.ProductValueAfter, review.ProductAfter),
		labeledAmount(view, i18n.ProductUndoNetWorth, delta, view.Colors.Ink),
	}
	for _, activity := range review.Activities {
		fields := []layout.Widget{}
		for _, effect := range activity.Effects {
			if effect.Money != nil {
				label := i18n.HoldingReconAmountRemoved
				if effect.Direction == domain.EffectAdded {
					label = i18n.HoldingReconAmountAdded
				}
				fields = append(fields, fundsAmount(view, label, effect.Money))
			}
		}
		rows = append(rows, fundsCard(view, l.Text(historyKindLabel(activity.Activity.Kind, activity.Activity.Reason)), fields...))
	}
	return rows
}
